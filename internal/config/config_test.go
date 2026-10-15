package config

import (
	"errors"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		wantErr bool
		want    Config // DataDir is checked separately
	}{
		{
			name: "defaults",
			want: Config{HTTPAddr: ":7777", LogLevel: slog.LevelInfo},
		},
		{
			name: "env values",
			env: map[string]string{
				"DNDVTT_HTTP_ADDR": ":9000", "DNDVTT_DEV": "1", "DNDVTT_LOG_LEVEL": "DEBUG",
			},
			want: Config{HTTPAddr: ":9000", Dev: true, LogLevel: slog.LevelDebug},
		},
		{
			name: "flag overrides env",
			args: []string{"-http-addr=:8000"},
			env:  map[string]string{"DNDVTT_HTTP_ADDR": ":9000"},
			want: Config{HTTPAddr: ":8000", LogLevel: slog.LevelInfo},
		},
		{
			name: "dev flag and log level flag",
			args: []string{"-dev", "-log-level=warn"},
			want: Config{HTTPAddr: ":7777", Dev: true, LogLevel: slog.LevelWarn},
		},
		{name: "bad DNDVTT_DEV", env: map[string]string{"DNDVTT_DEV": "maybe"}, wantErr: true},
		{name: "bad log level", args: []string{"-log-level=loud"}, wantErr: true},
		{name: "empty http addr", args: []string{"-http-addr="}, wantErr: true},
		{name: "unknown flag", args: []string{"-nope"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			args := append([]string{"-data-dir=" + dir}, tt.args...)
			got, err := Load(args, env(tt.env))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.HTTPAddr != tt.want.HTTPAddr || got.Dev != tt.want.Dev || got.LogLevel != tt.want.LogLevel {
				t.Errorf("Load = %+v, want %+v", got, tt.want)
			}
			if !filepath.IsAbs(got.DataDir) {
				t.Errorf("DataDir %q is not absolute", got.DataDir)
			}
			if st, err := os.Stat(got.DataDir); err != nil || !st.IsDir() {
				t.Errorf("DataDir %q does not exist as a directory: %v", got.DataDir, err)
			}
		})
	}
}

func TestLoadDataDirFromEnv(t *testing.T) {
	dir := t.TempDir()
	got, err := Load(nil, env(map[string]string{"DNDVTT_DATA_DIR": dir}))
	if err != nil {
		t.Fatal(err)
	}
	if got.DataDir != dir {
		t.Errorf("DataDir = %q, want %q", got.DataDir, dir)
	}
}

func TestLoadDevDefaultDataDir(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	got, err := Load([]string{"-dev"}, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	// Resolve symlinks (e.g. macOS /var -> /private/var) before comparing.
	wantDir, err := filepath.EvalSymlinks(filepath.Join(cwd, "data"))
	if err != nil {
		t.Fatalf("data dir was not created: %v", err)
	}
	gotDir, err := filepath.EvalSymlinks(got.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if gotDir != wantDir {
		t.Errorf("DataDir = %q, want %q", gotDir, wantDir)
	}
}

func TestLoadHelp(t *testing.T) {
	_, err := Load([]string{"-h"}, env(nil))
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("err = %v, want flag.ErrHelp", err)
	}
}

func TestLoadOpenBrowser(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		want    bool
		wantErr bool
	}{
		{name: "default", want: true},
		{name: "dev disables it", args: []string{"-dev"}, want: false},
		{name: "env overrides dev default", args: []string{"-dev"}, env: map[string]string{"DNDVTT_OPEN_BROWSER": "1"}, want: true},
		{name: "flag overrides env", args: []string{"-open-browser=false"}, env: map[string]string{"DNDVTT_OPEN_BROWSER": "1"}, want: false},
		{name: "flag enables it in dev", args: []string{"-dev", "-open-browser"}, want: true},
		{name: "bad env", env: map[string]string{"DNDVTT_OPEN_BROWSER": "maybe"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"-data-dir=" + t.TempDir()}, tt.args...)
			got, err := Load(args, env(tt.env))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got.OpenBrowser != tt.want {
				t.Errorf("OpenBrowser = %v, want %v", got.OpenBrowser, tt.want)
			}
		})
	}
}
