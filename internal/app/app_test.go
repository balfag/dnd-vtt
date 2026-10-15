package app

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/balfag/dnd-vtt/internal/config"
)

func TestRunServesAndStops(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- New(config.Config{}, logger, "test").Run(ctx, ln) }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
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
			conn.Close()
			t.Fatal("server still accepts connections after Run returned")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s after cancel")
	}
}
