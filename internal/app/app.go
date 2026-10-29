// Package app wires the server components together and runs them.
package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/coder/websocket"

	"github.com/balfag/dnd-vtt/internal/asset"
	"github.com/balfag/dnd-vtt/internal/auth"
	"github.com/balfag/dnd-vtt/internal/chat"
	"github.com/balfag/dnd-vtt/internal/config"
	"github.com/balfag/dnd-vtt/internal/dice"
	"github.com/balfag/dnd-vtt/internal/game"
	"github.com/balfag/dnd-vtt/internal/httpapi"
	"github.com/balfag/dnd-vtt/internal/vtt"
	"github.com/balfag/dnd-vtt/internal/wiki"
	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/web"
)

const (
	shutdownTimeout = 10 * time.Second
	trashPurgeEvery = time.Hour
)

// App is the server application.
type App struct {
	cfg     config.Config
	logger  *slog.Logger
	version string
	auth    *auth.Service
	game    *game.Service
	hub     *ws.Hub
	assets  *asset.Service
	vtt     *vtt.Service
	wiki    *wiki.Service
	chat    *chat.Service

	out         io.Writer              // where the sign-in link is printed; os.Stdout in New
	openBrowser func(url string) error // openURL in New
}

// New assembles the application on an opened and migrated database.
// It neither listens nor starts anything.
func New(cfg config.Config, logger *slog.Logger, version string, db *sql.DB) (*App, error) {
	authSvc, err := auth.NewService(db, auth.Options{})
	if err != nil {
		return nil, fmt.Errorf("create auth service: %w", err)
	}
	gameSvc := game.NewService(db, game.Options{})
	if cfg.Dev {
		created, err := gameSvc.EnsureDevCampaign(context.Background())
		if err != nil {
			return nil, fmt.Errorf("ensure dev campaign: %w", err)
		}
		if created {
			logger.Info("dev campaign created")
		}
	}
	hub := ws.NewHub(gameSvc, logger, ws.Options{})
	vttSvc := vtt.NewService(db, vtt.Options{})
	vtt.Register(hub, vttSvc, dice.CryptoRoller())
	hub.AddSnapshotSource(vttSvc)
	chatSvc := chat.NewService(db, chat.Options{})
	chat.Register(hub, chatSvc, dice.CryptoRoller())
	hub.AddSnapshotSource(chatSvc)
	return &App{
		cfg:         cfg,
		logger:      logger,
		version:     version,
		auth:        authSvc,
		game:        gameSvc,
		hub:         hub,
		assets:      asset.NewService(db, cfg.DataDir, asset.Options{}),
		vtt:         vttSvc,
		wiki:        wiki.NewService(db, wiki.Options{}),
		chat:        chatSvc,
		out:         os.Stdout,
		openBrowser: openURL,
	}, nil
}

// Run serves HTTP on ln until ctx is cancelled, then shuts down gracefully
// with a 10 second timeout. A clean stop returns nil.
func (a *App) Run(ctx context.Context, ln net.Listener) error {
	// No Read/WriteTimeout: long-lived WebSocket connections will live here.
	srv := &http.Server{
		Handler: httpapi.NewRouter(httpapi.Deps{
			Logger:  a.logger,
			Version: a.version,
			SPA:     web.Dist(),
			Auth:    a.auth,
			Game:    a.game,
			Hub:     a.hub,
			Assets:  a.assets,
			VTT:     a.vtt,
			Wiki:    a.wiki,
			Chat:    a.chat,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// The purge goroutine stops with Run, whichever way it returns.
	purgeCtx, stopPurge := context.WithCancel(ctx)
	purgeDone := make(chan struct{})
	go func() {
		defer close(purgeDone)
		a.purgeTrashLoop(purgeCtx, trashPurgeEvery)
	}()
	defer func() {
		stopPurge()
		<-purgeDone
	}()

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	a.logger.Info("http server started", "addr", ln.Addr().String())
	// After Serve is running, so the browser never opens before the server is ready.
	a.announce(ctx, ln)

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve http: %w", err)
	case <-ctx.Done():
	}

	// Shutdown does not touch hijacked WebSocket connections.
	a.hub.CloseAll(websocket.StatusGoingAway, "server shutting down")

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

// purgeTrashLoop deletes expired wiki trash at once and then every interval
// until ctx is done. A failure is logged and the loop goes on.
func (a *App) purgeTrashLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		n, err := a.wiki.PurgeExpired(ctx)
		switch {
		case err != nil:
			if ctx.Err() == nil {
				a.logger.Error("purge wiki trash", "err", err)
			}
		case n > 0:
			a.logger.Info("wiki trash purged", "roots", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// announce issues a setup token (no GM account yet) or a one-time GM token,
// prints the link to a.out and opens it in the browser if a.cfg.OpenBrowser.
// The link carries a secret, so it goes to stdout only, never to the logger.
func (a *App) announce(ctx context.Context, ln net.Listener) {
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		a.logger.Error("issue sign-in link", "err", fmt.Errorf("unexpected listener address %T", ln.Addr()))
		return
	}
	base := fmt.Sprintf("http://localhost:%d", tcp.Port)

	exists, err := a.auth.GMExists(ctx)
	if err != nil {
		a.logger.Error("issue sign-in link", "err", err)
		return
	}

	var url string
	if !exists {
		token, err := a.auth.IssueSetupToken()
		if err != nil {
			a.logger.Error("issue sign-in link", "err", err)
			return
		}
		url = base + "/setup?t=" + token
		_, _ = fmt.Fprintf(a.out, "First run: create the GM account at %s\n", url)
	} else {
		token, err := a.auth.IssueGMToken()
		if err != nil {
			a.logger.Error("issue sign-in link", "err", err)
			return
		}
		url = base + "/?gm=" + token
		_, _ = fmt.Fprintf(a.out, "Sign in as GM on this computer (link valid 10 minutes): %s\n", url)
	}

	if a.cfg.OpenBrowser {
		if err := a.openBrowser(url); err != nil {
			// The URL holds a token, so it is not logged.
			a.logger.Warn("open browser", "err", err)
		}
	}
}
