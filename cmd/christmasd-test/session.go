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
	*chi.Mux
	ledCoords []image.Point
	logger    *slog.Logger

	sessions sync2.Map[string, *sessionInstance]
}

func newSessionManager(logger *slog.Logger, ledPoints []image.Point) *sessionsHandler {
	h := &sessionsHandler{
		ledCoords: ledPoints,
		logger:    logger,
	}

	h.Get("/", h.handleNewSession)
	h.Get("/{token}/ws", h.handleSessionWS)

	return h
}

func (m *sessionsHandler) handleNewSession(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
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
		buffer: leddraw.NewLEDStripBuffer(len(canvas.LEDs())),
		rctx:   r.Context(),
	}

	token := m.addSession(session)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	init := controllerEventToSSE(ControllerInit{
		LEDCoords:    m.ledCoords,
		SessionToken: token,
	})
	writeSSE(w, init)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-session.frame:
			session.canvasMu.Lock()
			frame := controllerEventToSSE(ControllerFrame{
				LEDColors: session.canvas.LEDs(),
			})
			session.canvasMu.Unlock()
			writeSSE(w, frame)
		}
	}
}

func (m *sessionsHandler) handleSessionWS(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	session, ok := m.sessions.Load(token)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	christmasSession, err := christmasd.SessionUpgrade(w, r, serverConfig, christmasd.ServerOpts{
		LEDController: (*sessionLEDController)(session),
		Logger:        m.logger.With("token", token),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := christmasSession.Start(r.Context()); err != nil {
		m.logger.Warn(
			"session ended with error",
			"token", token,
			"error", err)
	}
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
	buffer   *leddraw.LEDStripBuffer
	rctx     context.Context
}

type sessionLEDController sessionInstance

var _ christmasd.LEDController = (*sessionLEDController)(nil)

func (c *sessionLEDController) LEDs() leddraw.LEDStrip {
	c.canvasMu.Lock()
	defer c.canvasMu.Unlock()

	c.buffer.SetLEDs(c.canvas.LEDs())
	return c.buffer.LEDs()
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
