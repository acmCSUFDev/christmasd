package main

import (
	"context"
	"embed"
	"fmt"
	"image"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"

	"dev.acmcsuf.com/christmas/lib/csvutil"
	"dev.acmcsuf.com/christmasd"
	"github.com/go-chi/chi/v5"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"github.com/spf13/pflag"
	"libdb.so/hserve"
)

//go:generate deno bundle frontend/script.ts frontend/script.js
//go:embed frontend
var frontendFS embed.FS

var (
	httpAddr     = ":9001"
	ledPointsCSV = "led-points.csv"
	canvasPPI    = 72.0
	verbose      = false
)

func init() {
	pflag.StringVarP(&httpAddr, "http-addr", "a", httpAddr, "HTTP server address")
	pflag.StringVar(&ledPointsCSV, "led-points", ledPointsCSV, "CSV file of LED points")
	pflag.Float64Var(&canvasPPI, "canvas-ppi", canvasPPI, "canvas PPI")
	pflag.BoolVarP(&verbose, "verbose", "v", verbose, "verbose logging")
}

const frameRate = 20

var serverConfig = christmasd.Config{}

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

	manager := newSessionManager(logger, ledCoords)

	r := chi.NewRouter()
	r.Mount("/sessions", manager)
	r.Mount("/", http.FileServer(http.FS(frontendFS)))

	return hserve.ListenAndServe(ctx, httpAddr, r)
}
