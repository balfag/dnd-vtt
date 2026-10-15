// Package app wires the server components together and runs them.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/balfag/dnd-vtt/internal/config"
	"github.com/balfag/dnd-vtt/internal/httpapi"
)

const shutdownTimeout = 10 * time.Second

// App is the server application.
type App struct {
	cfg     config.Config
	logger  *slog.Logger
	version string
}

// New assembles the application. It neither listens nor starts anything.
func New(cfg config.Config, logger *slog.Logger, version string) *App {
	return &App{cfg: cfg, logger: logger, version: version}
}

// Run serves HTTP on ln until ctx is cancelled, then shuts down gracefully
// with a 10 second timeout. A clean stop returns nil.
func (a *App) Run(ctx context.Context, ln net.Listener) error {
	// No Read/WriteTimeout: long-lived WebSocket connections will live here.
	srv := &http.Server{
		Handler:           httpapi.NewRouter(a.logger, a.version),
		ReadHeaderTimeout: 10 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	a.logger.Info("http server started", "addr", ln.Addr().String())

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve http: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}
	// Wait for Serve to return so no goroutine outlives Run.
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve http: %w", err)
	}
	a.logger.Info("http server stopped")
	return nil
}
