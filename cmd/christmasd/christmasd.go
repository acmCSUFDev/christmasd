package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"image"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"

	"dev.acmcsuf.com/christmas/lib/csvutil"
	"dev.acmcsuf.com/christmasd"
	"github.com/go-chi/chi/v5"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"
	"libdb.so/hserve"
	"libdb.so/ledctl"
)

var (
	httpAddr      = "0.0.0.0:9000"
	httpAdminAddr = "127.0.0.1:9002"
	ledPointsCSV  = "led-points.csv"
	canvasPPI     = 72.0
	verbose       = false
)

func init() {
	pflag.StringVarP(&httpAddr, "http-addr", "a", httpAddr, "HTTP server address")
	pflag.StringVarP(&httpAdminAddr, "http-admin-addr", "A", httpAdminAddr, "HTTP admin server address")
	pflag.StringVar(&ledPointsCSV, "led-points", ledPointsCSV, "CSV file of LED points")
	pflag.Float64Var(&canvasPPI, "canvas-ppi", canvasPPI, "canvas PPI")
	pflag.BoolVarP(&verbose, "verbose", "v", verbose, "verbose logging")
}

const frameRate = 20

var ws281xConfig = ledctl.WS281xConfig{
	ColorOrder:   ledctl.BGROrder,
	ColorModel:   ledctl.RGBModel,
	PWMFrequency: 800000,
	DMAChannel:   10,
	GPIOPins:     []int{12},
}

func main() {
	log.SetFlags(0)
	pflag.Parse()

	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	logHandler := tint.NewHandler(os.Stderr, &tint.Options{
		Level:      level,
		TimeFormat: "15:04:05 PM", // extended time.Kitchen
		NoColor:    !isatty.IsTerminal(os.Stderr.Fd()),
	})

	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := run(ctx, logger); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	ledCoords, err := csvutil.UnmarshalFile[image.Point](ledPointsCSV)
	if err != nil {
		return fmt.Errorf("failed to unmarshal CSV file %q: %v", ledPointsCSV, err)
	}

	ws281xCfg := ws281xConfig
	ws281xCfg.NumPixels = len(ledCoords)

	ws281x, err := ledctl.NewWS281x(ws281xCfg)
	if err != nil {
		return fmt.Errorf("failed to create a WS281x controller: %v", err)
	}

	controller, err := newLEDController(ledControlConfig{
		Controller: ws281x,
		LEDCoords:  ledCoords,
		FrameRate:  frameRate,
		CanvasPPI:  canvasPPI,
		Logger:     logger.With("component", "led-controller"),
	})
	if err != nil {
		return fmt.Errorf("failed to create a LED controller: %v", err)
	}

	server := christmasd.NewServer(christmasd.ServerOpts{
		LEDController: controller,
		Logger:        logger.With("component", "server"),
	})

	token := atomic.Pointer[string]{}

	errg, ctx := errgroup.WithContext(ctx)
	errg.Go(func() error {
		r := chi.NewRouter()
		r.Get("/ws/{token}", func(w http.ResponseWriter, r *http.Request) {
			tokenWant := token.Load()
			token := chi.URLParam(r, "token")
			if tokenWant != nil {
				if token != *tokenWant {
					http.Error(w, "invalid token", http.StatusForbidden)
					return
				}
			}

			server.ServeHTTP(w, r)
		})

		r.Get("/led-points.csv", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/csv")
			w.Header().Set("Content-Disposition", "attachment; filename=led-points.csv")

			csvw := csv.NewWriter(w)
			csvutil.Marshal(csvw, ledCoords)
		})

		logger.Info(
			"starting public HTTP server",
			"addr", httpAddr)

		return hserve.ListenAndServe(ctx, httpAddr, r)
	})

	errg.Go(func() error {
		admin := newAdminHandler(server, &token)

		logger.Info(
			"starting admin HTTP server",
			"addr", httpAdminAddr)

		return hserve.ListenAndServe(ctx, httpAdminAddr, admin)
	})

	return errg.Wait()
}
