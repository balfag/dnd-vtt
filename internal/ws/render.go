package ws

import (
	"encoding/json"
	"strconv"

	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// Viewer is who an event is rendered for.
type Viewer struct {
	IsGM     bool
	PlayerID string // "" for a GM who is not a participant of the campaign
}

// Renderer returns the event as v must see it; ok=false means v gets sync.noop.
// It runs under Hub.mu at emit time and again on resume, so it must be a pure
// function of v and of data captured when the event was emitted: no I/O, no
// shared mutable state.
type Renderer func(v Viewer) (t string, d any, ok bool)

// Same is a Renderer that gives everyone the same event. To emit an event to
// everyone use Emitter.EmitAll: its frame is serialized once, while a
// Renderer is called for every recipient and on every resume.
func Same(t string, d any) Renderer {
	return func(Viewer) (string, any, bool) { return t, d, true }
}

// noopFrame is the frame of an event that has nothing for the recipient.
func noopFrame(seq int64) []byte {
	return []byte(`{"t":"` + protocol.TypeNoop + `","seq":` + strconv.FormatInt(seq, 10) + `}`)
}

// marshalEvent encodes {t, seq, d}.
func marshalEvent(t string, seq int64, d any) ([]byte, error) {
	env, err := protocol.New(t, d)
	if err != nil {
		return nil, err
	}
	env.Seq = seq
	return json.Marshal(env)
}

// frameLocked returns the frame of ev for v: the stored one for an event for
// everyone, otherwise the event rendered anew. An event that cannot be
// encoded, or has nothing for v, becomes sync.noop so its number is not lost.
func (h *Hub) frameLocked(ev bufferedEvent, v Viewer) []byte {
	if ev.frame != nil {
		return ev.frame
	}
	t, d, ok := ev.render(v)
	if !ok {
		return noopFrame(ev.seq)
	}
	frame, err := marshalEvent(t, ev.seq, d)
	if err != nil {
		h.logger.Error("ws: encode rendered event", "type", t, "seq", ev.seq, "err", err)
		return noopFrame(ev.seq)
	}
	return frame
}

// viewerLocked is who cn is for rendering; it reads playerID, so mu is held.
func viewerLocked(cn *conn) Viewer {
	return Viewer{IsGM: cn.acc.IsGM, PlayerID: cn.playerID}
}

// emitLocked assigns the next seq to the event r describes, keeps it with r in
// the buffer (dropping events beyond BufferSize or older than BufferTTL) and
// queues to every subscribed connection its own rendering of it.
func (h *Hub) emitLocked(r Renderer) {
	if h.state.id == "" {
		return
	}
	h.appendEventLocked(bufferedEvent{seq: h.state.seq + 1, at: h.opts.Now(), render: r})
}

// emitSameLocked is emitLocked for an event everyone sees the same: the frame
// is serialized once and kept in the buffer.
func (h *Hub) emitSameLocked(t string, d any) {
	if h.state.id == "" {
		return
	}
	ev := bufferedEvent{seq: h.state.seq + 1, at: h.opts.Now()}
	frame, err := marshalEvent(t, ev.seq, d)
	if err != nil {
		h.logger.Error("ws: encode event", "type", t, "seq", ev.seq, "err", err)
		frame = noopFrame(ev.seq)
	}
	ev.frame = frame
	h.appendEventLocked(ev)
}

// appendEventLocked buffers ev and sends it to the subscribed connections.
func (h *Hub) appendEventLocked(ev bufferedEvent) {
	h.state.seq = ev.seq
	h.state.events = append(h.state.events, ev)
	h.trimBufferLocked()
	for cn := range h.conns {
		if cn.subscribed {
			cn.sendRaw(h.frameLocked(ev, viewerLocked(cn)))
		}
	}
}

// broadcastLocked emits an event of type t for everyone.
func (h *Hub) broadcastLocked(t string, d any) {
	h.emitSameLocked(t, d)
}
