package main

import (
	"bufio"
	"crypto/sha1" //nolint:gosec // required by RFC 6455 handshake, not used for security
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// A minimal RFC 6455 server-side WebSocket: text messages only, which is all
// the signalling needs. Written by hand because the spike may depend only on
// pion modules.

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

const maxWSMessage = 1 << 20

type wsConn struct {
	c   net.Conn
	br  *bufio.Reader
	wmu sync.Mutex
}

func upgradeWS(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") ||
		!strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
		http.Error(w, "websocket upgrade required", http.StatusBadRequest)
		return nil, errors.New("not a websocket upgrade")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return nil, errors.New("missing Sec-WebSocket-Key")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return nil, errors.New("hijack unsupported")
	}
	conn, brw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	sum := sha1.Sum([]byte(key + wsGUID)) //nolint:gosec
	accept := base64.StdEncoding.EncodeToString(sum[:])
	_, err = brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + accept + "\r\n\r\n")
	if err == nil {
		err = brw.Flush()
	}
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &wsConn{c: conn, br: brw.Reader}, nil
}

// readMessage returns the next text message, answering pings on the way.
func (w *wsConn) readMessage() ([]byte, error) {
	var msg []byte
	for {
		var h [2]byte
		if _, err := io.ReadFull(w.br, h[:]); err != nil {
			return nil, err
		}
		fin := h[0]&0x80 != 0
		op := h[0] & 0x0f
		masked := h[1]&0x80 != 0
		n := uint64(h[1] & 0x7f)
		switch n {
		case 126:
			var b [2]byte
			if _, err := io.ReadFull(w.br, b[:]); err != nil {
				return nil, err
			}
			n = uint64(binary.BigEndian.Uint16(b[:]))
		case 127:
			var b [8]byte
			if _, err := io.ReadFull(w.br, b[:]); err != nil {
				return nil, err
			}
			n = binary.BigEndian.Uint64(b[:])
		}
		if n > maxWSMessage {
			return nil, errors.New("websocket message too large")
		}
		var mask [4]byte
		if masked {
			if _, err := io.ReadFull(w.br, mask[:]); err != nil {
				return nil, err
			}
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(w.br, payload); err != nil {
			return nil, err
		}
		if masked {
			for i := range payload {
				payload[i] ^= mask[i%4]
			}
		}
		switch op {
		case 0x8:
			return nil, io.EOF
		case 0x9:
			if err := w.write(0xA, payload); err != nil {
				return nil, err
			}
		case 0xA:
		case 0x0, 0x1:
			msg = append(msg, payload...)
			if len(msg) > maxWSMessage {
				return nil, errors.New("websocket message too large")
			}
			if fin {
				return msg, nil
			}
		}
	}
}

func (w *wsConn) writeText(b []byte) error { return w.write(0x1, b) }

func (w *wsConn) write(op byte, payload []byte) error {
	w.wmu.Lock()
	defer w.wmu.Unlock()
	hdr := []byte{0x80 | op}
	switch n := len(payload); {
	case n < 126:
		hdr = append(hdr, byte(n))
	case n < 1<<16:
		hdr = append(hdr, 126, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, 127)
		hdr = binary.BigEndian.AppendUint64(hdr, uint64(n))
	}
	_ = w.c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := w.c.Write(hdr); err != nil {
		return err
	}
	_, err := w.c.Write(payload)
	return err
}

func (w *wsConn) close() { _ = w.c.Close() }
