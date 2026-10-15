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
	sendQueueLen    = 64
	writeTimeout    = 10 * time.Second
)

// Options tunes a Hub. Zero values mean the production defaults.
type Options struct {
	Now         func() time.Time // default time.Now
	IdleTimeout time.Duration    // default 60s: no message from the client -> close 4008
	RatePerSec  float64          // default 50: sustained messages per second
	RateBurst   float64          // default 100
	RateGrace   time.Duration    // default 5s: a run of rejections this long -> close 4029
	RateQuiet   time.Duration    // default 1s: a gap between rejections this long ends the run
}

// Hub tracks the WebSocket connections of the installation. Safe for concurrent use.
type Hub struct {
	game   *game.Service
	logger *slog.Logger
	opts   Options

	mu    sync.Mutex
	conns map[*conn]struct{}
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
	return &Hub{game: g, logger: logger, opts: opts, conns: make(map[*conn]struct{})}
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

	cn := newConn(ctx, cancel, h, c, acc, sess, playerID, sendQueueLen)
	h.register(cn)
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

	var wg sync.WaitGroup
	for _, cn := range all {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cn.c.Close(code, reason)
		}()
	}
	wg.Wait()
}

func (h *Hub) register(cn *conn) {
	h.mu.Lock()
	h.conns[cn] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) unregister(cn *conn) {
	h.mu.Lock()
	delete(h.conns, cn)
	h.mu.Unlock()
}

// online returns the ids of the participants that have at least one connection.
func (h *Hub) online() map[string]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[string]bool, len(h.conns))
	for cn := range h.conns {
		if cn.playerID != "" {
			out[cn.playerID] = true
		}
	}
	return out
}

// snapshot builds sync.snapshot for cn from the database and the hub's connections.
func (h *Hub) snapshot(ctx context.Context, cn *conn) (protocol.Snapshot, error) {
	snap := protocol.Snapshot{
		Me: protocol.SnapshotMe{AccountID: cn.acc.ID, IsGM: cn.acc.IsGM},
	}
	if cn.playerID != "" {
		id := cn.playerID
		snap.Me.PlayerID = &id
	}

	gs, err := h.game.ActiveGameSession(ctx)
	if errors.Is(err, game.ErrNoActiveGame) {
		return snap, nil
	}
	if err != nil {
		return protocol.Snapshot{}, err
	}
	camp, err := h.game.GetCampaign(ctx, gs.CampaignID)
	if err != nil {
		return protocol.Snapshot{}, err
	}
	players, err := h.game.ListPlayers(ctx, gs.CampaignID)
	if err != nil {
		return protocol.Snapshot{}, err
	}

	online := h.online()
	infos := make([]protocol.PlayerInfo, 0, len(players))
	for _, p := range players {
		if p.Removed {
			continue
		}
		infos = append(infos, protocol.PlayerInfo{
			ID:          p.ID,
			DisplayName: p.DisplayName,
			Color:       p.Color,
			Role:        p.Role,
			Online:      online[p.ID],
		})
	}
	snap.Game = &protocol.SnapshotGame{
		GameSessionID: gs.ID,
		StartedAt:     gs.StartedAt.UnixMilli(),
		Campaign:      protocol.CampaignRef{ID: camp.ID, Name: camp.Name},
		Players:       infos,
	}
	return snap, nil
}
