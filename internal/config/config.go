// Package config loads the server configuration from flags and environment.
package config

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
)

// Config is the runtime configuration of the server.
type Config struct {
	HTTPAddr string     // HTTP listen address
	DataDir  string     // data directory (absolute after Load)
	Dev      bool       // development mode
	LogLevel slog.Level // minimum log level
}

const defaultHTTPAddr = ":7777"

// Load reads the configuration. Priority: flag > environment variable > default.
// getenv is a parameter for tests; main passes os.Getenv.
// On -h/-help the returned error satisfies errors.Is(err, flag.ErrHelp).
// Load creates DataDir.
func Load(args []string, getenv func(string) string) (Config, error) {
	httpAddrDefault := defaultHTTPAddr
	if v := getenv("DNDVTT_HTTP_ADDR"); v != "" {
		httpAddrDefault = v
	}

	devDefault := false
	if v := getenv("DNDVTT_DEV"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid DNDVTT_DEV %q: %w", v, err)
		}
		devDefault = b
	}

	logLevelDefault := "info"
	if v := getenv("DNDVTT_LOG_LEVEL"); v != "" {
		logLevelDefault = v
	}

	fs := flag.NewFlagSet("dndvtt", flag.ContinueOnError)
	httpAddr := fs.String("http-addr", httpAddrDefault, "HTTP listen address")
	dataDir := fs.String("data-dir", getenv("DNDVTT_DATA_DIR"), "data directory")
	dev := fs.Bool("dev", devDefault, "development mode")
	logLevel := fs.String("log-level", logLevelDefault, "log level: debug, info, warn, error")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	if *httpAddr == "" {
		return Config{}, errors.New("http address must not be empty")
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(*logLevel)); err != nil {
		return Config{}, fmt.Errorf("invalid log level %q: %w", *logLevel, err)
	}

	dir := *dataDir
	if dir == "" {
		if *dev {
			dir = "data"
		} else {
			base, err := os.UserConfigDir()
			if err != nil {
				return Config{}, fmt.Errorf("determine user config dir: %w", err)
			}
			dir = filepath.Join(base, "dndvtt")
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve data dir %q: %w", dir, err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return Config{}, fmt.Errorf("create data dir %q: %w", abs, err)
	}

	return Config{HTTPAddr: *httpAddr, DataDir: abs, Dev: *dev, LogLevel: level}, nil
}
