package lan

import (
	"bufio"
	"crypto/tls"
	"net"
	"sync"
	"time"
)

// tlsRecordHandshake is the first byte of a TLS handshake record (ClientHello).
const tlsRecordHandshake = 0x16

// peekTimeout is how long a new connection may stay silent before it is closed.
var peekTimeout = 10 * time.Second

// NewListener wraps ln: each accepted connection whose first byte is 0x16
// (TLS handshake record) is returned as tls.Server(conn, cfg); any other
// connection is returned as is, with the peeked byte not lost. Peeking
// happens in a goroutine per connection with a 10 s read deadline (reset
// after the peek), so a silent client never blocks Accept. A connection that
// sends nothing in 10 s or fails the peek is closed and never returned.
// Close closes ln and makes Accept return net.ErrClosed.
func NewListener(ln net.Listener, cfg *tls.Config) net.Listener {
	l := &muxListener{
		Listener: ln,
		cfg:      cfg,
		conns:    make(chan net.Conn),
		failed:   make(chan struct{}),
		done:     make(chan struct{}),
	}
	go l.acceptLoop()
	return l
}

type muxListener struct {
	net.Listener
	cfg   *tls.Config
	conns chan net.Conn
	done  chan struct{} // closed by Close
	// failed is closed after err is set by the first failure of the underlying Accept.
	failed chan struct{}
	err    error
	once   sync.Once
}

// peekedConn reads through r so the peeked byte is not lost.
type peekedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *peekedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

func (l *muxListener) acceptLoop() {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			l.err = err
			close(l.failed)
			return
		}
		go l.sniff(conn)
	}
}

func (l *muxListener) sniff(conn net.Conn) {
	if err := conn.SetReadDeadline(time.Now().Add(peekTimeout)); err != nil {
		_ = conn.Close()
		return
	}
	r := bufio.NewReader(conn)
	first, err := r.Peek(1)
	if err != nil || conn.SetReadDeadline(time.Time{}) != nil {
		_ = conn.Close()
		return
	}
	var out net.Conn = &peekedConn{Conn: conn, r: r}
	if first[0] == tlsRecordHandshake {
		out = tls.Server(out, l.cfg)
	}
	select {
	case l.conns <- out:
	case <-l.done:
		_ = out.Close()
	}
}

// Accept returns the next connection, TLS or plain.
func (l *muxListener) Accept() (net.Conn, error) {
	select {
	case <-l.done:
		return nil, net.ErrClosed
	default:
	}
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.failed:
		return nil, l.err
	case <-l.done:
		return nil, net.ErrClosed
	}
}

// Close closes the underlying listener; it is safe to call more than once.
func (l *muxListener) Close() error {
	var err error
	l.once.Do(func() {
		close(l.done)
		err = l.Listener.Close()
	})
	return err
}
