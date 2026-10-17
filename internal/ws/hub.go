// Package ws serves the WebSocket channel of docs/architecture/protocol.md.
package ws

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/balfag/dnd-vtt/internal/auth"
	"github.com/balfag/dnd-vtt/internal/game"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

const (
	maxMessageBytes = 64 << 10
	// sendQueueSlack is the room of a connection's send queue beyond a full
	// resume buffer: replies, presence and the like.
	sendQueueSlack = 64
	writeTimeout   = 10 * time.Second
)

// Options tunes a Hub. Zero values mean the production defaults.
type Options struct {
	Now         func() time.Time // default time.Now
	IdleTimeout time.Duration    // default 60s: no message from the client -> close 4008
	RatePerSec  float64          // default 50: sustained messages per second
	RateBurst   float64          // default 100
	RateGrace   time.Duration    // default 5s: a run of rejections this long -> close 4029
	RateQuiet   time.Duration    // default 1s: a gap between rejections this long ends the run
	BufferSize  int              // default 1000: events kept for resume
	BufferTTL   time.Duration    // default 5m: older events are dropped from the buffer
}

// Hub tracks the WebSocket connections of the installation. Safe for concurrent use.
type Hub struct {
	game   *game.Service
	logger *slog.Logger
	opts   Options

	// mu guards conns, state and the playerID/subscribed fields of the
	// connections. Everything that assigns a seq, queues an event or builds a
	// snapshot runs under it, so no event can slip between reading the
	// snapshot from the database and getting its seq. Never write to a socket
	// or call Close under mu: queue frames with send/sendRaw only.
	mu    sync.Mutex
	conns map[*conn]struct{}
	state gameState

	// cmdMu serializes table commands: the change in the database and the seq
	// of its events do not interleave with another command. Lock order: cmdMu,
	// then mu, never the other way round; a handler runs without mu.
	cmdMu    sync.Mutex
	handlers map[string]handlerEntry

	// snapshotSources are read-only once the hub serves connections.
	snapshotSources []SnapshotSource
}

// gameState is the event stream of the active game session.
type gameState struct {
	id         string // active game session id, "" when there is no game
	campaignID string
	startedAt  time.Time
	seq        int64           // last assigned seq, 0 before the first event
	events     []bufferedEvent // ascending seq
}

// bufferedEvent is an event kept for resume. An event for everyone (Same)
// keeps its frame, serialized once; any other keeps the Renderer, so a resume
// renders it anew for the resuming connection.
type bufferedEvent struct {
	seq    int64
	at     time.Time
	frame  []byte   // marshalled envelope, nil for a rendered event
	render Renderer // nil for an event with a frame
}

// NewHub returns a Hub that reads game state from g.
func NewHub(g *game.Service, logger *slog.Logger, opts Options) *Hub {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = 60 * time.Second
	}
	if opts.RatePerSec <= 0 {
		opts.RatePerSec = 50
	}
	if opts.RateBurst <= 0 {
		opts.RateBurst = 100
	}
	if opts.RateGrace <= 0 {
		opts.RateGrace = 5 * time.Second
	}
	if opts.RateQuiet <= 0 {
		opts.RateQuiet = time.Second
	}
	if opts.BufferSize <= 0 {
		opts.BufferSize = 1000
	}
	if opts.BufferTTL <= 0 {
		opts.BufferTTL = 5 * time.Minute
	}
	return &Hub{game: g, logger: logger, opts: opts, conns: make(map[*conn]struct{}), handlers: make(map[string]handlerEntry)}
}

// Serve upgrades r to a WebSocket and runs the connection until it closes.
// The caller has already authenticated the request: acc and sess come from
// auth.ResolveSession. Origin is checked by websocket.Accept.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, acc auth.Account, sess auth.Session) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		// Accept has already answered the client (for example 403 for a foreign Origin).
		return
	}
	defer func() { _ = c.CloseNow() }()
	c.SetReadLimit(maxMessageBytes)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	playerID, closeCode, reason, err := h.participant(ctx, acc, sess)
	if err != nil {
		h.logger.Error("ws: find participant", "err", err)
		_ = c.Close(websocket.StatusInternalError, "internal error")
		return
	}
	if closeCode != 0 {
		_ = c.Close(closeCode, reason)
		return
	}

	cn := newConn(ctx, cancel, h, c, acc, sess, playerID, h.opts.BufferSize+sendQueueSlack)
	h.register(ctx, cn)
	defer h.unregister(cn)

	writerDone := make(chan struct{})
	go cn.writeLoop(writerDone)
	defer func() {
		cancel()
		<-writerDone
	}()

	idle := time.AfterFunc(h.opts.IdleTimeout, func() { _ = c.Close(protocol.CloseIdle, "idle") })
	defer idle.Stop()

	cn.readLoop(idle)
}

// participant resolves the campaign participant for the connecting account.
// A non-zero code means the connection must be closed with it.
func (h *Hub) participant(ctx context.Context, acc auth.Account, sess auth.Session) (playerID string, code websocket.StatusCode, reason string, err error) {
	gs, err := h.game.ActiveGameSession(ctx)
	hasGame := err == nil
	if err != nil && !errors.Is(err, game.ErrNoActiveGame) {
		return "", 0, "", err
	}

	if !acc.IsGM {
		if !hasGame || gs.ID != sess.GameSessionID {
			return "", protocol.CloseGameEnded, "game ended", nil
		}
		p, err := h.game.PlayerForAccount(ctx, gs.CampaignID, acc.ID)
		if errors.Is(err, game.ErrNotFound) {
			return "", protocol.ClosePlayerRemoved, "removed", nil
		}
		if err != nil {
			return "", 0, "", err
		}
		if p.Removed {
			return "", protocol.ClosePlayerRemoved, "removed", nil
		}
		return p.ID, 0, "", nil
	}

	if !hasGame {
		return "", 0, "", nil
	}
	p, err := h.game.PlayerForAccount(ctx, gs.CampaignID, acc.ID)
	if errors.Is(err, game.ErrNotFound) {
		return "", 0, "", nil
	}
	if err != nil {
		return "", 0, "", err
	}
	return p.ID, 0, "", nil
}

// CloseAll closes every connection with code and reason; used on shutdown
// (http.Server.Shutdown does not touch hijacked WebSocket connections).
func (h *Hub) CloseAll(code websocket.StatusCode, reason string) {
	h.mu.Lock()
	all := make([]*conn, 0, len(h.conns))
	for cn := range h.conns {
		all = append(all, cn)
	}
	h.mu.Unlock()
	closeConns(all, code, reason)
}

// closeConns closes the connections in parallel: a close waits for the
// peer's close frame. It must not be called with h.mu held. Hooks call it in
// a goroutine; CloseAll waits for it on shutdown.
func closeConns(conns []*conn, code websocket.StatusCode, reason string) {
	var wg sync.WaitGroup
	for _, cn := range conns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cn.c.Close(code, reason)
		}()
	}
	wg.Wait()
}

// register adds cn to the hub; the first connection of a participant
// announces it online.
func (h *Hub) register(ctx context.Context, cn *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[cn] = struct{}{}
	if cn.playerID == "" || h.countPlayerLocked(cn.playerID) != 1 {
		return
	}
	if err := h.syncGameLocked(ctx); err != nil {
		h.logger.Error("ws: sync game state", "err", err)
		return
	}
	h.broadcastLocked(protocol.TypePresence, protocol.Presence{PlayerID: cn.playerID, Online: true})
}

// unregister removes cn; the last connection of a participant announces it offline.
func (h *Hub) unregister(cn *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns, cn)
	if cn.playerID == "" || h.countPlayerLocked(cn.playerID) != 0 {
		return
	}
	h.broadcastLocked(protocol.TypePresence, protocol.Presence{PlayerID: cn.playerID, Online: false})
}

func (h *Hub) countPlayerLocked(playerID string) int {
	n := 0
	for cn := range h.conns {
		if cn.playerID == playerID {
			n++
		}
	}
	return n
}

// Online reports which participants have at least one connection.
func (h *Hub) Online() map[string]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.onlineLocked()
}

func (h *Hub) onlineLocked() map[string]bool {
	out := make(map[string]bool, len(h.conns))
	for cn := range h.conns {
		if cn.playerID != "" {
			out[cn.playerID] = true
		}
	}
	return out
}

// syncGameLocked makes h.state follow the active game in the database:
// a different game (or none) resets id, campaignID, seq and the buffer.
func (h *Hub) syncGameLocked(ctx context.Context) error {
	gs, err := h.game.ActiveGameSession(ctx)
	if errors.Is(err, game.ErrNoActiveGame) {
		h.state = gameState{}
		return nil
	}
	if err != nil {
		return err
	}
	if gs.ID != h.state.id {
		h.state = gameState{id: gs.ID, campaignID: gs.CampaignID, startedAt: gs.StartedAt}
	}
	return nil
}

// trimBufferLocked drops events beyond BufferSize and older than BufferTTL.
func (h *Hub) trimBufferLocked() {
	ev := h.state.events
	if n := len(ev) - h.opts.BufferSize; n > 0 {
		ev = ev[n:]
	}
	cutoff := h.opts.Now().Add(-h.opts.BufferTTL)
	for len(ev) > 0 && ev[0].at.Before(cutoff) {
		ev = ev[1:]
	}
	h.state.events = ev
}

// canResumeLocked reports whether a client that has seen events up to
// hello.LastSeq of game hello.GameSessionID can get the rest from the buffer.
func (h *Hub) canResumeLocked(hello protocol.Hello) bool {
	if hello.GameSessionID == "" || hello.GameSessionID != h.state.id {
		return false
	}
	if hello.LastSeq < 0 || hello.LastSeq > h.state.seq {
		return false
	}
	if hello.LastSeq == h.state.seq {
		return true
	}
	h.trimBufferLocked()
	ev := h.state.events
	return len(ev) > 0 && ev[0].seq <= hello.LastSeq+1
}

// snapshotLocked builds sync.snapshot for cn; Seq is h.state.seq. The caller
// has just called syncGameLocked.
func (h *Hub) snapshotLocked(ctx context.Context, cn *conn) (protocol.Snapshot, error) {
	snap := protocol.Snapshot{
		Seq: h.state.seq,
		Me:  protocol.SnapshotMe{AccountID: cn.acc.ID, IsGM: cn.acc.IsGM},
	}
	if cn.playerID != "" {
		id := cn.playerID
		snap.Me.PlayerID = &id
	}
	if h.state.id == "" {
		return snap, nil
	}

	camp, err := h.game.GetCampaign(ctx, h.state.campaignID)
	if err != nil {
		return protocol.Snapshot{}, err
	}
	players, err := h.game.ListPlayers(ctx, h.state.campaignID)
	if err != nil {
		return protocol.Snapshot{}, err
	}

	online := h.onlineLocked()
	infos := make([]protocol.PlayerInfo, 0, len(players))
	for _, p := range players {
		if p.Removed {
			continue
		}
		infos = append(infos, playerInfo(p, online[p.ID]))
	}
	// An empty table first; the sources fill their parts of it in order.
	table := protocol.SnapshotTable{Scenes: []protocol.SceneListItem{}, Chat: []protocol.ChatMessage{}}
	for _, src := range h.snapshotSources {
		if err := src.FillSnapshot(ctx, viewerLocked(cn), h.state.campaignID, &table); err != nil {
			return protocol.Snapshot{}, err
		}
	}
	snap.Game = &protocol.SnapshotGame{
		GameSessionID: h.state.id,
		StartedAt:     h.state.startedAt.UnixMilli(),
		Campaign:      protocol.CampaignRef{ID: camp.ID, Name: camp.Name},
		Players:       infos,
		SnapshotTable: table,
	}
	return snap, nil
}

// SnapshotSource fills its part of the M1 table in sync.snapshot. It runs
// under Hub.mu while the snapshot is built and may read the database, but must
// not call the Hub.
type SnapshotSource interface {
	FillSnapshot(ctx context.Context, v Viewer, campaignID string, t *protocol.SnapshotTable) error
}

// AddSnapshotSource appends s; sources run in the order they were added.
// Call before serving.
func (h *Hub) AddSnapshotSource(s SnapshotSource) {
	h.snapshotSources = append(h.snapshotSources, s)
}

func playerInfo(p game.Player, online bool) protocol.PlayerInfo {
	return protocol.PlayerInfo{
		ID:          p.ID,
		DisplayName: p.DisplayName,
		Color:       p.Color,
		Role:        p.Role,
		Online:      online,
	}
}

// GameStarted is called after a game session starts (the previous one, if
// any, has ended). Players of the previous game are closed with 4004; GM
// connections switch to the new game and get a fresh snapshot.
func (h *Hub) GameStarted(ctx context.Context) { h.gameChanged(ctx) }

// GameEnded is called after the active game session ends: players are closed
// with 4004, GM connections get a fresh snapshot without a game.
func (h *Hub) GameEnded(ctx context.Context) { h.gameChanged(ctx) }

func (h *Hub) gameChanged(ctx context.Context) {
	h.mu.Lock()
	toClose := h.gameChangedLocked(ctx)
	h.mu.Unlock()
	// Do not wait for the close handshakes: a silent client would hold the
	// HTTP request that triggered the hook for up to 5 seconds.
	go closeConns(toClose, protocol.CloseGameEnded, "game ended")
}

// gameChangedLocked detaches the players' connections (the caller closes
// them) and moves the GM connections to the current game.
func (h *Hub) gameChangedLocked(ctx context.Context) []*conn {
	// Detach first: a closing connection must not announce itself offline
	// into the new game's event stream.
	var toClose []*conn
	for cn := range h.conns {
		if !cn.acc.IsGM {
			cn.playerID = ""
			cn.subscribed = false
			toClose = append(toClose, cn)
		}
	}
	if err := h.syncGameLocked(ctx); err != nil {
		h.logger.Error("ws: sync game state", "err", err)
	}
	for cn := range h.conns {
		if !cn.acc.IsGM {
			continue
		}
		cn.playerID = h.gmPlayerIDLocked(ctx, cn)
		if !cn.subscribed {
			continue
		}
		snap, err := h.snapshotLocked(ctx, cn)
		if err != nil {
			h.logger.Error("ws: build snapshot", "err", err)
			continue
		}
		cn.sendNew(protocol.TypeSnapshot, "", snap)
	}
	return toClose
}

// gmPlayerIDLocked returns the GM's participant in the current game, or "".
func (h *Hub) gmPlayerIDLocked(ctx context.Context, cn *conn) string {
	if h.state.id == "" {
		return ""
	}
	p, err := h.game.PlayerForAccount(ctx, h.state.campaignID, cn.acc.ID)
	if errors.Is(err, game.ErrNotFound) {
		return ""
	}
	if err != nil {
		h.logger.Error("ws: find gm participant", "err", err)
		return ""
	}
	return p.ID
}

// PlayerJoined announces a participant that registered or joined during the
// game: presence.update with the player field (online = current state).
func (h *Hub) PlayerJoined(ctx context.Context, p game.Player) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.syncGameLocked(ctx); err != nil {
		h.logger.Error("ws: sync game state", "err", err)
		return
	}
	if p.CampaignID != h.state.campaignID || p.Removed {
		return
	}
	online := h.onlineLocked()[p.ID]
	info := playerInfo(p, online)
	h.broadcastLocked(protocol.TypePresence, protocol.Presence{PlayerID: p.ID, Online: online, Player: &info})
}

// PlayerRemoved closes the participant's connections with 4003 and announces
// it offline once.
func (h *Hub) PlayerRemoved(playerID string) {
	h.mu.Lock()
	var toClose []*conn
	for cn := range h.conns {
		if cn.playerID == playerID {
			cn.playerID = ""
			cn.subscribed = false
			toClose = append(toClose, cn)
		}
	}
	if len(toClose) > 0 {
		h.broadcastLocked(protocol.TypePresence, protocol.Presence{PlayerID: playerID, Online: false})
	}
	h.mu.Unlock()
	go closeConns(toClose, protocol.ClosePlayerRemoved, "removed")
}

// CloseSession closes connections of one login session with 4001 (logout).
func (h *Hub) CloseSession(sessionID string) {
	h.closeMatching(func(cn *conn) bool { return cn.sess.ID == sessionID })
}

// CloseAccountSessions closes the account's connections with 4001, except
// those of exceptSessionID (password change keeps the current device).
func (h *Hub) CloseAccountSessions(accountID, exceptSessionID string) {
	h.closeMatching(func(cn *conn) bool { return cn.acc.ID == accountID && cn.sess.ID != exceptSessionID })
}

func (h *Hub) closeMatching(match func(*conn) bool) {
	h.mu.Lock()
	var toClose []*conn
	for cn := range h.conns {
		if match(cn) {
			toClose = append(toClose, cn)
		}
	}
	h.mu.Unlock()
	go closeConns(toClose, protocol.CloseSessionRevoked, "session revoked")
}
