package main

import (
	"context"
	"image"
	"log/slog"
	"net/http"
	"sync"

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
			"error", err)

		http.Error(w, "failed to create LED canvas", http.StatusInternalServerError)
		return
	}

	session := &sessionInstance{
		frame:  make(chan struct{}, 1),
		canvas: canvas,
		buffer: make(leddraw.LEDStrip, len(canvas.LEDs())),
		rctx:   r.Context(),
	}

	token := m.addSession(session)

	m.logger.Info(
		"new session created",
		"token", token)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	init := controllerEventToSSE(ControllerInit{
		LEDCoords:    m.ledCoords,
		SessionToken: token,
	})
	writeSSE(wflush, init)

frameLoop:
	for {
		select {
		case <-r.Context().Done():
			break frameLoop
		case <-session.frame:
			session.canvasMu.Lock()
			frame := controllerEventToSSE(ControllerFrame{
				LEDColors: session.canvas.LEDs(),
			})
			session.canvasMu.Unlock()
			writeSSE(wflush, frame)

			m.logger.Info(
				"session frame sent",
				"token", token)
		}
	}

	m.logger.Info(
		"session has been closed",
		"token", token)
}

func (m *sessionsHandler) handleSessionWS(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	session, ok := m.sessions.Load(token)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

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
