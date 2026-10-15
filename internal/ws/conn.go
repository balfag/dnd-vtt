package ws

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"

	"github.com/balfag/dnd-vtt/internal/auth"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// conn is one WebSocket connection. Only the read loop touches helloDone and
// the limiter state; out is the only way to write to the socket.
type conn struct {
	hub  *Hub
	c    *websocket.Conn
	acc  auth.Account
	sess auth.Session

	// playerID and subscribed are guarded by hub.mu.
	playerID   string // "" for a GM outside a game
	subscribed bool   // gets events after the snapshot or the resume

	ctx    context.Context
	cancel context.CancelFunc
	out    chan []byte

	helloDone bool

	tokens     float64
	lastRefill time.Time
	runStart   time.Time // start of the current run of rejections; zero: no run
	lastReject time.Time
}

func newConn(ctx context.Context, cancel context.CancelFunc, h *Hub, c *websocket.Conn, acc auth.Account, sess auth.Session, playerID string, queueLen int) *conn {
	return &conn{
		hub:        h,
		c:          c,
		acc:        acc,
		sess:       sess,
		playerID:   playerID,
		ctx:        ctx,
		cancel:     cancel,
		out:        make(chan []byte, queueLen),
		tokens:     h.opts.RateBurst,
		lastRefill: h.opts.Now(),
	}
}

// writeLoop is the only goroutine that writes to the socket.
func (cn *conn) writeLoop(done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-cn.ctx.Done():
			return
		case b := <-cn.out:
			wctx, cancel := context.WithTimeout(cn.ctx, writeTimeout)
			err := cn.c.Write(wctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				cn.cancel()
				return
			}
		}
	}
}

// readLoop reads messages until the connection ends. Every message resets idle.
func (cn *conn) readLoop(idle *time.Timer) {
	for {
		typ, data, err := cn.c.Read(cn.ctx)
		if err != nil {
			return
		}
		if typ != websocket.MessageText {
			_ = cn.c.Close(websocket.StatusUnsupportedData, "text frames only")
			return
		}
		idle.Reset(cn.hub.opts.IdleTimeout)
		if !cn.handle(data) {
			return
		}
	}
}

// send queues env for the writer goroutine. A full queue means the client
// does not read fast enough: the connection is closed with 1013.
func (cn *conn) send(env protocol.Envelope) {
	b, err := json.Marshal(env)
	if err != nil {
		cn.hub.logger.Error("ws: marshal envelope", "type", env.T, "err", err)
		return
	}
	cn.sendRaw(b)
}

// sendRaw queues an already marshalled frame; a full queue closes the connection with 1013.
func (cn *conn) sendRaw(b []byte) {
	select {
	case cn.out <- b:
	default:
		go func() { _ = cn.c.Close(websocket.StatusTryAgainLater, "client too slow") }()
	}
}

// sendNew queues an envelope of type t with data d (nil d: no data) and
// answers command re.
func (cn *conn) sendNew(t, re string, d any) {
	env, err := protocol.New(t, d)
	if err != nil {
		cn.hub.logger.Error("ws: encode data", "type", t, "err", err)
		return
	}
	env.Re = re
	cn.send(env)
}

func (cn *conn) sendError(re, code, message string) {
	env, err := protocol.ErrorReply(re, code, message)
	if err != nil {
		cn.hub.logger.Error("ws: encode error reply", "err", err)
		return
	}
	cn.send(env)
}

// allow spends one token of the rate limiter. firstInRun reports that this
// rejection started a new run of rejections; exceeded reports that the run
// lasted RateGrace and the connection must be closed.
func (cn *conn) allow(now time.Time) (ok, firstInRun, exceeded bool) {
	o := cn.hub.opts
	if dt := now.Sub(cn.lastRefill); dt > 0 {
		cn.tokens += o.RatePerSec * dt.Seconds()
		if cn.tokens > o.RateBurst {
			cn.tokens = o.RateBurst
		}
		cn.lastRefill = now
	}
	if cn.tokens >= 1 {
		cn.tokens--
		return true, false, false
	}
	// Passed messages do not end a run: with a steady flood a token appears
	// every 1/RatePerSec, and the run would never reach RateGrace.
	if cn.runStart.IsZero() || now.Sub(cn.lastReject) >= o.RateQuiet {
		cn.runStart = now
		firstInRun = true
	}
	cn.lastReject = now
	return false, firstInRun, now.Sub(cn.runStart) >= o.RateGrace
}

// handle processes one text frame. It returns false when the read loop must stop.
func (cn *conn) handle(data []byte) bool {
	// Parse first only to learn the command id; every frame, valid or not,
	// is counted by the rate limiter so garbage cannot bypass it.
	var env protocol.Envelope
	parseErr := json.Unmarshal(data, &env)

	ok, firstInRun, exceeded := cn.allow(cn.hub.opts.Now())
	if !ok {
		// One reply per run: answering every rejection would amplify a flood.
		if firstInRun {
			cn.sendError(env.ID, protocol.CodeRateLimited, "too many messages")
		}
		if exceeded {
			_ = cn.c.Close(protocol.CloseRateLimited, "rate limited")
			return false
		}
		return true
	}
	if parseErr != nil || env.T == "" {
		cn.sendError(env.ID, protocol.CodeBadRequest, "invalid envelope")
		return true
	}

	switch {
	case env.T == protocol.TypePing:
		cn.send(protocol.Envelope{T: protocol.TypePong, Re: env.ID})
	case env.T == protocol.TypeHello:
		cn.handleHello(env)
	case !cn.helloDone:
		cn.sendError(env.ID, protocol.CodeBadRequest, "sync.hello expected")
	default:
		cn.sendError(env.ID, protocol.CodeBadRequest, "unknown message type")
	}
	return true
}

func (cn *conn) handleHello(env protocol.Envelope) {
	var hello protocol.Hello
	if len(env.D) > 0 {
		if err := json.Unmarshal(env.D, &hello); err != nil {
			cn.sendError(env.ID, protocol.CodeBadRequest, "invalid hello data")
			return
		}
	}

	h := cn.hub
	// The snapshot (or the resume) and the subscription happen under one lock,
	// so no event falls between the state the client gets and the stream.
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.syncGameLocked(cn.ctx); err != nil {
		h.logger.Error("ws: sync game state", "err", err)
		cn.sendError(env.ID, protocol.CodeInternal, "internal error")
		return
	}

	if h.canResumeLocked(hello) {
		if env.ID != "" {
			cn.sendNew(protocol.TypeAck, env.ID, protocol.HelloAck{Resumed: true})
		}
		for _, ev := range h.state.events {
			if ev.seq > hello.LastSeq {
				cn.sendRaw(ev.frame)
			}
		}
		cn.subscribed = true
		cn.helloDone = true
		return
	}

	snap, err := h.snapshotLocked(cn.ctx, cn)
	if err != nil {
		h.logger.Error("ws: build snapshot", "err", err)
		cn.sendError(env.ID, protocol.CodeInternal, "internal error")
		return
	}
	if env.ID != "" {
		cn.sendNew(protocol.TypeAck, env.ID, protocol.HelloAck{Resumed: false})
	}
	cn.sendNew(protocol.TypeSnapshot, "", snap)
	cn.subscribed = true
	cn.helloDone = true
}
