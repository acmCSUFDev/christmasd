package main

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"sync"
	"time"

	"dev.acmcsuf.com/christmas/lib/leddraw"
	"dev.acmcsuf.com/christmasd"
	"libdb.so/ledctl"
)

// RGBController is a controller for RGB LEDs.
type RGBController interface {
	SetRGBAt(i int, color ledctl.RGB)
	Flush() error
}

type ledController struct {
	canvas *leddraw.LEDCanvas
	logger *slog.Logger

	drawCh chan struct{}
	ctrl   RGBController
	ctrlMu sync.Mutex

	cfg ledControlConfig
}

var _ christmasd.LEDController = (*ledController)(nil)

type ledControlConfig struct {
	Controller RGBController
	LEDCoords  []image.Point
	FrameRate  int
	CanvasPPI  float64

	Logger *slog.Logger
}

func newLEDController(cfg ledControlConfig) (*ledController, error) {
	canvasOpts := leddraw.LEDCanvasOpts{PPI: cfg.CanvasPPI}
	canvas, err := leddraw.NewLEDCanvas(cfg.LEDCoords, canvasOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create LED canvas: %v", err)
	}

	return &ledController{
		canvas: canvas,
		logger: cfg.Logger,
		drawCh: make(chan struct{}, 1),
		ctrl:   cfg.Controller,
		cfg:    cfg,
	}, nil
}

func (c *ledController) start(ctx context.Context) {
	drawCh := c.drawCh

	frameTicker := time.NewTicker(time.Second / time.Duration(c.cfg.FrameRate))
	defer frameTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-frameTicker.C:
			drawCh = c.drawCh
			continue
		case <-drawCh:
			drawCh = nil
		}

		c.ctrlMu.Lock()
		if err := c.ctrl.Flush(); err != nil {
			c.logger.Error(
				"error writing LED strip",
				"error", err)
		}
		c.ctrlMu.Unlock()
	}
}

func (c *ledController) LEDs() leddraw.LEDStrip {
	return c.canvas.LEDs()
}

func (c *ledController) SetLEDs(strip leddraw.LEDStrip) error {
	c.ctrlMu.Lock()
	defer c.ctrlMu.Unlock()

	for i, color := range strip {
		c.ctrl.SetRGBAt(i, ledctl.RGB(color))
	}

	c.queueDraw()
	return nil
}

func (c *ledController) ImageSize() (w, h int) {
	bounds := c.canvas.CanvasBounds()
	return bounds.Dx(), bounds.Dy()
}

func (c *ledController) DrawImage(img *image.RGBA) error {
	if err := c.canvas.Render(img); err != nil {
		return fmt.Errorf("failed to render image: %v", err)
	}
	return c.SetLEDs(c.canvas.LEDs())
}

func (c *ledController) queueDraw() {
	select {
	case c.drawCh <- struct{}{}:
	default:
	}
}
