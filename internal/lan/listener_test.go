package lan

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// startMuxServer serves a handler that reports whether the request was TLS.
func startMuxServer(t *testing.T) (addr string, ln net.Listener) {
	t.Helper()
	cert, _, err := LoadOrCreateCert(t.TempDir(), nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln = NewListener(raw, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.TLS != nil {
				_, _ = io.WriteString(w, "tls")
				return
			}
			_, _ = io.WriteString(w, "plain")
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return raw.Addr().String(), ln
}

func get(t *testing.T, client *http.Client, url string) string {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, %v", url, resp.StatusCode, err)
	}
	return string(body)
}

func TestListenerServesHTTPAndHTTPS(t *testing.T) {
	addr, _ := startMuxServer(t)
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test
		},
	}
	t.Cleanup(client.CloseIdleConnections)

	if got := get(t, client, "http://"+addr+"/"); got != "plain" {
		t.Errorf("http body = %q, want plain", got)
	}
	if got := get(t, client, "https://"+addr+"/"); got != "tls" {
		t.Errorf("https body = %q, want tls", got)
	}
}

func TestListenerSilentClientDoesNotBlock(t *testing.T) {
	addr, _ := startMuxServer(t)
	silent, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = silent.Close() })

	client := &http.Client{Timeout: 5 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	if got := get(t, client, "http://"+addr+"/"); got != "plain" {
		t.Errorf("body = %q, want plain", got)
	}
}

func TestListenerCloseUnblocksAccept(t *testing.T) {
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln := NewListener(raw, &tls.Config{MinVersion: tls.VersionTLS12})

	errc := make(chan error, 1)
	go func() {
		_, err := ln.Accept()
		errc <- err
	}()
	time.Sleep(50 * time.Millisecond)
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ln.Close(); err != nil {
		t.Errorf("second Close = %v", err)
	}
	select {
	case err := <-errc:
		if !errors.Is(err, net.ErrClosed) {
			t.Errorf("Accept error = %v, want net.ErrClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Accept did not return after Close")
	}
	if _, err := ln.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Errorf("Accept after Close = %v, want net.ErrClosed", err)
	}
}

type tempErr struct{}

func (tempErr) Error() string   { return "temporary" }
func (tempErr) Temporary() bool { return true }

// flakyListener fails with a temporary error n times, then returns conn.
type flakyListener struct {
	net.Listener
	fails int
	conn  net.Conn
}

func (f *flakyListener) Accept() (net.Conn, error) {
	if f.fails > 0 {
		f.fails--
		return nil, tempErr{}
	}
	return f.conn, nil
}

func (f *flakyListener) Close() error { return nil }

func TestAcceptRetriesTemporaryErrors(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = client.Close() }()
	ln := NewListener(&flakyListener{fails: 2, conn: server}, &tls.Config{MinVersion: tls.VersionTLS12})
	defer func() { _ = ln.Close() }()
	go func() { _, _ = client.Write([]byte("G")) }()
	conn, err := ln.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	defer func() { _ = conn.Close() }()
	buf := make([]byte, 1)
	if _, err := io.ReadFull(conn, buf); err != nil || buf[0] != 'G' {
		t.Fatalf("read = %q, %v", buf, err)
	}
}
