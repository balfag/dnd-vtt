// Package protocol defines the WebSocket messages of docs/architecture/protocol.md.
package protocol

import "encoding/json"

// Envelope is one WebSocket text frame.
type Envelope struct {
	T   string          `json:"t"`
	ID  string          `json:"id,omitempty"`
	Re  string          `json:"re,omitempty"`
	Seq int64           `json:"seq,omitempty"`
	D   json.RawMessage `json:"d,omitempty"`
}

// Message types of M0.
const (
	TypeHello    = "sync.hello"
	TypeSnapshot = "sync.snapshot"
	TypePresence = "presence.update"
	TypeNoop     = "sync.noop" // the number is taken, there is nothing for this recipient
	TypePing     = "ping"
	TypePong     = "pong"
	TypeAck      = "ack"
	TypeError    = "error"
)

// Error codes carried in ErrorData.Code.
const (
	CodeBadRequest  = "bad_request"
	CodeForbidden   = "forbidden"
	CodeNotFound    = "not_found"
	CodeConflict    = "conflict"
	CodeRateLimited = "rate_limited"
	CodeInternal    = "internal"
)

// Close codes of the application (RFC 6455 4000-4999 range).
const (
	CloseSessionRevoked = 4001
	ClosePlayerRemoved  = 4003
	CloseGameEnded      = 4004
	CloseIdle           = 4008
	CloseRateLimited    = 4029
)

// Hello is the data of sync.hello. Both fields are optional.
type Hello struct {
	GameSessionID string `json:"gameSessionId,omitempty"`
	LastSeq       int64  `json:"lastSeq,omitempty"`
}

// HelloAck is the data of the ack to sync.hello.
type HelloAck struct {
	Resumed bool `json:"resumed"`
}

// ErrorData is the data of an error reply.
type ErrorData struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Snapshot is the data of sync.snapshot.
type Snapshot struct {
	Seq  int64         `json:"seq"`
	Me   SnapshotMe    `json:"me"`
	Game *SnapshotGame `json:"game"` // null: no game session (GM only)
}

// SnapshotMe describes the account the snapshot is for.
type SnapshotMe struct {
	AccountID string  `json:"accountId"`
	IsGM      bool    `json:"isGm"`
	PlayerID  *string `json:"playerId"` // null: GM outside a game
}

// SnapshotGame describes the active game session.
type SnapshotGame struct {
	GameSessionID string       `json:"gameSessionId"`
	StartedAt     int64        `json:"startedAt"` // unix milliseconds
	Campaign      CampaignRef  `json:"campaign"`
	Players       []PlayerInfo `json:"players"` // never null; removed participants are not listed
}

// CampaignRef is the campaign of a game session.
type CampaignRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PlayerInfo is a campaign participant as other clients see it.
type PlayerInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Color       string `json:"color"`
	Role        string `json:"role"`
	Online      bool   `json:"online"`
}

// Presence is the data of presence.update. Player is set only when a new
// participant joins during the game.
type Presence struct {
	PlayerID string      `json:"playerId"`
	Online   bool        `json:"online"`
	Player   *PlayerInfo `json:"player,omitempty"`
}

// New returns an envelope of type t whose D is d encoded as JSON.
// A nil d leaves D empty.
func New(t string, d any) (Envelope, error) {
	env := Envelope{T: t}
	if d == nil {
		return env, nil
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return Envelope{}, err
	}
	env.D = raw
	return env, nil
}

// ErrorReply returns an error envelope answering command re.
func ErrorReply(re, code, message string) (Envelope, error) {
	env, err := New(TypeError, ErrorData{Code: code, Message: message})
	if err != nil {
		return Envelope{}, err
	}
	env.Re = re
	return env, nil
}
