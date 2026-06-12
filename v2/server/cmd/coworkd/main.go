// Command coworkd is the CoWork OS server entrypoint (B/S, Go).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coworkos/cowork-os/v2/server/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db pool init failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())

	// Liveness + DB readiness probe.
	e.GET("/healthz", func(c echo.Context) error {
		pingCtx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "db_unavailable"})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	go func() {
		if err := e.Start(cfg.Addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped", "err", err)
			os.Exit(1)
		}
	}()
	logger.Info("coworkd started", "addr", cfg.Addr)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
	}
}
