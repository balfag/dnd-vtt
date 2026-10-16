package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// Command is a table command of a subscribed connection in the active game.
type Command struct {
	T             string
	ID            string          // "" for a command without a reply
	D             json.RawMessage // nil when absent
	Viewer        Viewer
	AccountID     string
	CampaignID    string
	GameSessionID string
}

// CommandError makes the command fail with an error reply of Code.
type CommandError struct{ Code, Message string }

func (e *CommandError) Error() string { return "ws: " + e.Code + ": " + e.Message }

// Errorf returns a *CommandError with a formatted message.
func Errorf(code, format string, args ...any) *CommandError {
	return &CommandError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Emitter publishes events of a command. Safe to call only during the handler.
type Emitter interface {
	// Emit assigns the next seq and sends r to every subscribed connection.
	Emit(r Renderer)
	// EmitAll is Emit(Same(t, d)).
	EmitAll(t string, d any)
	// Transient sends {t, d} without seq to subscribed connections whose
	// viewer passes to (nil: all); nothing is buffered.
	Transient(t string, d any, to func(Viewer) bool)
}

// Handler runs one command; a non-nil ack is the data of the ack reply.
type Handler func(ctx context.Context, cmd Command, em Emitter) (ack any, err error)

type HandlerOptions struct {
	RatePerSec float64 // > 0: per-connection limit for this type (burst = RatePerSec)
	Silent     bool    // over that limit: drop without a reply
}

type handlerEntry struct {
	fn   Handler
	opts HandlerOptions
}

// Handle registers fn for message type t. Register all handlers before the
// hub serves connections. It panics on a duplicate type, an empty type or a
// type of M0 (sync.*, ping, pong, ack, error) — that is a programming error.
func (h *Hub) Handle(t string, fn Handler, opts HandlerOptions) {
	switch {
	case t == "":
		panic("ws: Handle: empty message type")
	case strings.HasPrefix(t, "sync."), t == protocol.TypePing, t == protocol.TypePong,
		t == protocol.TypeAck, t == protocol.TypeError:
		panic("ws: Handle: " + t + " is a message type of the base protocol")
	}
	if _, dup := h.handlers[t]; dup {
		panic("ws: Handle: " + t + " is already registered")
	}
	h.handlers[t] = handlerEntry{fn: fn, opts: opts}
}

// typeBucket is the token bucket of one command type on one connection.
type typeBucket struct {
	tokens float64
	last   time.Time
}

// allowType spends a token of the limit of type t; always true without a limit.
func (cn *conn) allowType(t string, o HandlerOptions, now time.Time) bool {
	if o.RatePerSec <= 0 {
		return true
	}
	if cn.typeBuckets == nil {
		cn.typeBuckets = make(map[string]*typeBucket)
	}
	b, ok := cn.typeBuckets[t]
	if !ok {
		b = &typeBucket{tokens: o.RatePerSec, last: now}
		cn.typeBuckets[t] = b
	}
	if dt := now.Sub(b.last); dt > 0 {
		b.tokens = min(b.tokens+o.RatePerSec*dt.Seconds(), o.RatePerSec)
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// dispatch runs a registered command. Its rejection by the limit of the type
// is not a rejection of the general limit, so it never leads to a close.
func (cn *conn) dispatch(env protocol.Envelope, entry handlerEntry) {
	h := cn.hub
	if !cn.allowType(env.T, entry.opts, h.opts.Now()) {
		if !entry.opts.Silent && env.ID != "" {
			cn.sendError(env.ID, protocol.CodeRateLimited, "too many commands of this type")
		}
		return
	}

	// One command at a time; cmdMu is taken before mu and never under it.
	h.cmdMu.Lock()
	defer h.cmdMu.Unlock()

	h.mu.Lock()
	gameID, campaignID := h.state.id, h.state.campaignID
	viewer := viewerLocked(cn)
	subscribed := cn.subscribed
	h.mu.Unlock()
	if gameID == "" || !subscribed {
		cn.replyError(env.ID, protocol.CodeConflict, "no active game")
		return
	}

	cmd := Command{
		T:             env.T,
		ID:            env.ID,
		D:             env.D,
		Viewer:        viewer,
		AccountID:     cn.acc.ID,
		CampaignID:    campaignID,
		GameSessionID: gameID,
	}
	em := &emitter{h: h, gameID: gameID}
	ack, err := cn.runHandler(entry.fn, cmd, em)
	em.done.Store(true)

	if err != nil {
		var cerr *CommandError
		if errors.As(err, &cerr) {
			cn.replyError(env.ID, cerr.Code, cerr.Message)
			return
		}
		h.logger.Error("ws: command failed", "type", env.T, "err", err)
		cn.replyError(env.ID, protocol.CodeInternal, "internal error")
		return
	}
	if env.ID != "" {
		cn.sendNew(protocol.TypeAck, env.ID, ack)
	}
}

// replyError answers a command with an error; a command without an id gets no reply.
func (cn *conn) replyError(id, code, message string) {
	if id != "" {
		cn.sendError(id, code, message)
	}
}

// runHandler calls fn and turns a panic into an error: a faulty handler must
// not take the server down.
func (cn *conn) runHandler(fn Handler, cmd Command, em Emitter) (ack any, err error) {
	defer func() {
		if r := recover(); r != nil {
			cn.hub.logger.Error("ws: command handler panicked", "type", cmd.T, "panic", r, "stack", string(debug.Stack()))
			ack, err = nil, fmt.Errorf("handler of %s panicked: %v", cmd.T, r)
		}
	}()
	return fn(cn.ctx, cmd, em)
}

// emitter is the Emitter of one command of the game gameID.
type emitter struct {
	h      *Hub
	gameID string
	done   atomic.Bool // the handler has returned
}

// live reports, under h.mu, whether the command's game is still the active one.
func (e *emitter) liveLocked() bool {
	return !e.done.Load() && e.h.state.id == e.gameID
}

func (e *emitter) Emit(r Renderer) {
	e.h.mu.Lock()
	defer e.h.mu.Unlock()
	if e.liveLocked() {
		e.h.emitLocked(r)
	}
}

func (e *emitter) EmitAll(t string, d any) {
	e.h.mu.Lock()
	defer e.h.mu.Unlock()
	if e.liveLocked() {
		e.h.emitSameLocked(t, d)
	}
}

func (e *emitter) Transient(t string, d any, to func(Viewer) bool) {
	env, err := protocol.New(t, d)
	if err != nil {
		e.h.logger.Error("ws: encode transient message", "type", t, "err", err)
		return
	}
	frame, err := json.Marshal(env)
	if err != nil {
		e.h.logger.Error("ws: marshal transient message", "type", t, "err", err)
		return
	}

	e.h.mu.Lock()
	defer e.h.mu.Unlock()
	if !e.liveLocked() {
		return
	}
	for cn := range e.h.conns {
		if cn.subscribed && (to == nil || to(viewerLocked(cn))) {
			cn.sendRaw(frame)
		}
	}
}
