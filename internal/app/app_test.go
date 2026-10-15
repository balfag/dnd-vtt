package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/balfag/dnd-vtt/internal/config"
	"github.com/balfag/dnd-vtt/internal/game"
	"github.com/balfag/dnd-vtt/internal/store"
)

func TestNewCreatesDevCampaign(t *testing.T) {
	for _, tt := range []struct {
		name string
		dev  bool
		want int
	}{
		{"dev mode", true, 1},
		{"normal mode", false, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			if _, err := New(config.Config{Dev: tt.dev}, logger, "test", st.DB()); err != nil {
				t.Fatal(err)
			}

			list, err := game.NewService(st.DB(), game.Options{}).ListCampaigns(context.Background())
			if err != nil || len(list) != tt.want {
				t.Fatalf("campaigns = %+v, %v; want %d", list, err, tt.want)
			}
			if tt.want == 1 && list[0].Name != "Dev campaign" {
				t.Errorf("name = %q, want Dev campaign", list[0].Name)
			}
		})
	}
}

func TestRunServesAndStops(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	a, err := New(config.Config{}, logger, "test", st.DB())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx, ln) }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
		if conn, err := net.DialTimeout("tcp", ln.Addr().String(), 500*time.Millisecond); err == nil {
			_ = conn.Close()
			t.Fatal("server still accepts connections after Run returned")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s after cancel")
	}
}

func TestAnnounce(t *testing.T) {
	// newApp builds an App on a temporary store with captured output, logs and
	// browser calls.
	type env struct {
		app    *App
		ln     net.Listener
		out    *bytes.Buffer
		logs   *bytes.Buffer
		opened *[]string
	}
	newApp := func(t *testing.T, openBrowser bool, openErr error) env {
		t.Helper()
		st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })

		logs := &bytes.Buffer{}
		a, err := New(config.Config{OpenBrowser: openBrowser}, slog.New(slog.NewTextHandler(logs, nil)), "test", st.DB())
		if err != nil {
			t.Fatal(err)
		}
		out := &bytes.Buffer{}
		opened := &[]string{}
		a.out = out
		a.openBrowser = func(u string) error {
			*opened = append(*opened, u)
			return openErr
		}

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ln.Close() })
		return env{app: a, ln: ln, out: out, logs: logs, opened: opened}
	}
	port := func(e env) int { return e.ln.Addr().(*net.TCPAddr).Port }

	t.Run("no GM yet", func(t *testing.T) {
		e := newApp(t, true, nil)
		e.app.announce(context.Background(), e.ln)

		prefix := fmt.Sprintf("http://localhost:%d/setup?t=", port(e))
		line := strings.TrimSuffix(e.out.String(), "\n")
		if strings.Count(e.out.String(), "\n") != 1 || !strings.HasPrefix(line, "First run: create the GM account at "+prefix) {
			t.Fatalf("out = %q", e.out.String())
		}
		url := strings.TrimPrefix(line, "First run: create the GM account at ")
		if len(*e.opened) != 1 || (*e.opened)[0] != url {
			t.Fatalf("opened = %v, want exactly [%s]", *e.opened, url)
		}
		token := strings.TrimPrefix(url, prefix)
		if _, err := e.app.auth.CreateGM(context.Background(), token, "master", "master-password"); err != nil {
			t.Errorf("the issued setup token was rejected: %v", err)
		}
		if strings.Contains(e.logs.String(), token) {
			t.Error("logs contain the setup token")
		}
	})

	t.Run("GM exists", func(t *testing.T) {
		e := newApp(t, false, nil)
		setup, err := e.app.auth.IssueSetupToken()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.app.auth.CreateGM(context.Background(), setup, "master", "master-password"); err != nil {
			t.Fatal(err)
		}
		e.app.announce(context.Background(), e.ln)

		prefix := fmt.Sprintf("http://localhost:%d/?gm=", port(e))
		line := strings.TrimSuffix(e.out.String(), "\n")
		const lead = "Sign in as GM on this computer (link valid 10 minutes): "
		if strings.Count(e.out.String(), "\n") != 1 || !strings.HasPrefix(line, lead+prefix) {
			t.Fatalf("out = %q", e.out.String())
		}
		if len(*e.opened) != 0 {
			t.Errorf("browser opened with OpenBrowser=false: %v", *e.opened)
		}
		token := strings.TrimPrefix(line, lead+prefix)
		if _, err := e.app.auth.RedeemGMToken(context.Background(), token); err != nil {
			t.Errorf("the issued GM token was rejected: %v", err)
		}
		if strings.Contains(e.logs.String(), token) {
			t.Error("logs contain the GM token")
		}
	})

	t.Run("browser failure", func(t *testing.T) {
		e := newApp(t, true, errors.New("no browser"))
		e.app.announce(context.Background(), e.ln) // must return normally

		if !strings.Contains(e.logs.String(), "open browser") {
			t.Errorf("logs = %q, want an 'open browser' warning", e.logs.String())
		}
		token := strings.SplitN(e.out.String(), "?t=", 2)[1]
		if strings.Contains(e.logs.String(), strings.TrimSpace(token)) {
			t.Error("logs contain the setup token")
		}
		if strings.Contains(e.logs.String(), "localhost:") {
			t.Error("logs contain the sign-in URL")
		}
	})
}
