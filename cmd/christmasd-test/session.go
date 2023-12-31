package main

import (
	"context"
	"errors"
	"image"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"

	"dev.acmcsuf.com/christmas/lib/leddraw"
	"dev.acmcsuf.com/christmasd"
	"github.com/go-chi/chi/v5"
	"github.com/gofrs/uuid/v5"
	"gopkg.in/typ.v4/sync2"
)

type sessionsHandler struct {
	ledCoords []image.Point
	logger    *slog.Logger

	sessions sync2.Map[string, *sessionInstance]
}

func (m *sessionsHandler) handleNewSession(w http.ResponseWriter, r *http.Request) {
	wflush, ok := w.(writeFlusher)
	if !ok {
		http.Error(w, "server does not support flushing", http.StatusInternalServerError)
		return
	}

	canvasOpts := leddraw.LEDCanvasOpts{PPI: canvasPPI}
	canvas, err := leddraw.NewLEDCanvas(m.ledCoords, canvasOpts)
	if err != nil {
		m.logger.Error(
			"failed to create LED canvas",
			"addr", r.RemoteAddr,
			"error", err)

		http.Error(w, "failed to create LED canvas", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), maxSessionTTL)
	defer cancel()

	session := &sessionInstance{
		frame:  make(chan struct{}, 1),
		canvas: canvas,
		buffer: make(leddraw.LEDStrip, len(canvas.LEDs())),
		rctx:   ctx,
	}

	token := m.addSession(session)

	m.logger.Info(
		"new session has been created",
		"addr", r.RemoteAddr,
		"token", token)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	init := controllerEventSSE(ControllerInit{
		LEDCoords:    m.ledCoords,
		SessionToken: token,
	})
	writeSSE(wflush, init)

frameLoop:
	for {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			var reason string
			switch {
			case errors.Is(err, context.Canceled):
				reason = "client closed connection"
			case errors.Is(err, context.DeadlineExceeded):
				reason = "session timed out, please reconnect"
			default:
				reason = "server error occurred, please reconnect"
			}
			writeSSE(wflush, controllerEventSSE(ControllerGoingAway{
				Reason: reason,
			}))
			break frameLoop

		case <-session.frame:
			session.canvasMu.Lock()
			frame := controllerEventSSE(ControllerFrame{
				LEDColors: session.canvas.LEDs(),
			})
			session.canvasMu.Unlock()
			writeSSE(wflush, frame)

			m.logger.Debug(
				"session frame sent",
				"token", token)
		}
	}

	m.logger.Info(
		"session has been closed",
		"addr", r.RemoteAddr,
		"token", token,
		"timedOut", errors.Is(ctx.Err(), context.DeadlineExceeded))
}

func (m *sessionsHandler) handleSessionWS(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	session, ok := m.sessions.Load(token)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	if !session.occupied.CompareAndSwap(false, true) {
		http.Error(w, "session already occupied", http.StatusConflict)
		return
	}
	defer session.occupied.Store(false)

	christmasSession, err := christmasd.SessionUpgrade(w, r, christmasd.ServerOpts{
		LEDController: (*sessionLEDController)(session),
		Logger:        m.logger.With("token", token),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	m.logger.Info(
		"session has been connected to a new websocket",
		"addr", r.RemoteAddr,
		"token", token)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		select {
		case <-ctx.Done():
		case <-session.rctx.Done():
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		if err := christmasSession.Start(ctx); err != nil {
			m.logger.Warn(
				"session ended with error",
				"token", token,
				"error", err)
		}
	}()

	wg.Wait()

	m.logger.Info(
		"session has been disconnected from websocket",
		"addr", r.RemoteAddr,
		"token", token)
}

func (h *sessionsHandler) addSession(s *sessionInstance) string {
	for {
		uuid, err := uuid.NewV7()
		if err != nil {
			panic(err)
		}

		token := uuid.String()
		if _, collided := h.sessions.LoadOrStore(token, s); !collided {
			return token
		}
	}
}

type sessionInstance struct {
	frame    chan struct{}
	canvasMu sync.Mutex
	canvas   *leddraw.LEDCanvas
	buffer   leddraw.LEDStrip
	rctx     context.Context

	occupied atomic.Bool
}

type sessionLEDController sessionInstance

var _ christmasd.LEDController = (*sessionLEDController)(nil)

func (c *sessionLEDController) LEDs() leddraw.LEDStrip {
	c.canvasMu.Lock()
	defer c.canvasMu.Unlock()

	copy(c.buffer, c.canvas.LEDs())
	return c.buffer
}

func (c *sessionLEDController) SetLEDs(leds leddraw.LEDStrip) error {
	c.canvasMu.Lock()
	defer c.canvasMu.Unlock()

	copy(c.canvas.LEDs(), leds)

	c.queueDraw()
	return nil
}

func (c *sessionLEDController) ImageSize() (w, h int) {
	bounds := c.canvas.CanvasBounds()
	return bounds.Dx(), bounds.Dy()
}

func (c *sessionLEDController) DrawImage(img *image.RGBA) error {
	c.canvasMu.Lock()
	defer c.canvasMu.Unlock()

	if err := c.canvas.Render(img); err != nil {
		return err
	}

	c.queueDraw()
	return nil
}

func (c *sessionLEDController) queueDraw() {
	select {
	case c.frame <- struct{}{}:
	default:
	}
}
