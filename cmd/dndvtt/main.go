// Command dndvtt is the D&D virtual tabletop server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/balfag/dnd-vtt/internal/app"
	"github.com/balfag/dnd-vtt/internal/config"
	"github.com/balfag/dnd-vtt/internal/store"
)

// version is set at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() { os.Exit(run()) }

// run holds all the logic so deferred calls fire before os.Exit.
func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(os.Args[1:], os.Getenv)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 2
	}

	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	var handler slog.Handler
	if cfg.Dev {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	logger.Info("starting", "version", version, "dataDir", cfg.DataDir, "dev", cfg.Dev)

	dbPath := filepath.Join(cfg.DataDir, "dndvtt.db")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		logger.Error("open database", "path", dbPath, "err", err)
		return 1
	}
	defer func() {
		if err := st.Close(); err != nil {
			logger.Error("close database", "err", err)
		}
	}()
	schemaVersion, err := st.SchemaVersion(ctx)
	if err != nil {
		logger.Error("read schema version", "err", err)
		return 1
	}
	logger.Info("database ready", "path", dbPath, "schemaVersion", schemaVersion)

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		logger.Error("listen", "addr", cfg.HTTPAddr, "err", err)
		return 1
	}

	if err := app.New(cfg, logger, version).Run(ctx, ln); err != nil {
		logger.Error("run", "err", err)
		return 1
	}
	return 0
}
