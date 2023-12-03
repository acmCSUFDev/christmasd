package main

import (
	"context"
	"embed"
	"encoding/csv"
	"fmt"
	"image"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"dev.acmcsuf.com/christmas/lib/csvutil"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"github.com/spf13/pflag"
	"libdb.so/hserve"
)

//go:generate deno bundle frontend/script/main.ts frontend/script.js
//go:embed frontend
var frontendFS embed.FS
var frontendFilesFS, _ = fs.Sub(frontendFS, "frontend")

var (
	httpAddr      = ":9001"
	ledPointsCSV  = "led-points.csv"
	maxSessions   = 500
	maxSessionTTL = 3 * time.Hour
	canvasPPI     = 72.0
	verbose       = false
)

func init() {
	pflag.StringVarP(&httpAddr, "http-addr", "a", httpAddr, "HTTP server address")
	pflag.StringVar(&ledPointsCSV, "led-points", ledPointsCSV, "CSV file of LED points")
	pflag.IntVar(&maxSessions, "max-sessions", maxSessions, "maximum number of sessions")
	pflag.DurationVar(&maxSessionTTL, "max-session-ttl", maxSessionTTL, "maximum session TTL")
	pflag.Float64Var(&canvasPPI, "canvas-ppi", canvasPPI, "canvas PPI")
	pflag.BoolVarP(&verbose, "verbose", "v", verbose, "verbose logging")
}

const frameRate = 20

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

	h := &sessionsHandler{
		ledCoords: ledCoords,
		logger:    logger,
	}

	r := chi.NewRouter()
	r.Use(middleware.RealIP)

	r.Get("/led-points.csv", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=led-points.csv")

		csvw := csv.NewWriter(w)
		csvutil.Marshal(csvw, ledCoords)
	})

	r.Group(func(r chi.Router) {
		// Combine this with maxSessionTTL to mitigate DoS attacks.
		// In the future, we'd want to delay reconnects as well by using a more
		// sophisticated rate limiter.
		r.Use(middleware.Throttle(maxSessions))

		r.Get("/session", h.handleNewSession)
		r.Get("/ws/{token}", h.handleSessionWS)
	})

	r.Mount("/", http.FileServer(http.FS(frontendFilesFS)))

	logger.Info(
		"starting HTTP server",
		"addr", httpAddr)

	return hserve.ListenAndServe(ctx, httpAddr, r)
}
