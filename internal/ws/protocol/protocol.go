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
	SnapshotTable
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

// Message types of the scenes of M1.
const (
	TypeSceneCreate    = "scene.create"
	TypeSceneUpdate    = "scene.update"
	TypeSceneDelete    = "scene.delete"
	TypeSceneReorder   = "scene.reorder"
	TypeSceneActivate  = "scene.activate"
	TypeSceneView      = "scene.view"
	TypeScenesChanged  = "scenes.changed"
	TypeSceneChanged   = "scene.changed"
	TypeSceneActivated = "scene.activated"
)

// Grid is the grid of a scene.
type Grid struct {
	Type    string  `json:"type"` // square | hexPointy | hexFlat
	SizePx  int     `json:"sizePx"`
	UnitFt  int     `json:"unitFt"`
	OffsetX float64 `json:"offsetX"`
	OffsetY float64 `json:"offsetY"`
	Color   string  `json:"color"`
	Visible bool    `json:"visible"`
}

// AssetRef points at an uploaded file.
type AssetRef struct {
	AssetID string `json:"assetId"`
	URL     string `json:"url"` // "/assets/<id>"
}

// Fog is the fog of a scene. Ops are passed through as stored until #51 types them.
type Fog struct {
	Enabled bool            `json:"enabled"`
	Ops     json.RawMessage `json:"ops"` // JSON array, "[]" for a new scene
}

// SceneView is a scene as a client sees it. Drawings join in #52.
type SceneView struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Width      int       `json:"width"`
	Height     int       `json:"height"`
	Grid       Grid      `json:"grid"`
	Background *AssetRef `json:"background"` // null: no background
	Fog        Fog       `json:"fog"`
	Tokens     []Token   `json:"tokens"` // visible to the viewer, by z then id; never null
}

// SceneInfo is a scene without its tokens and drawings: the data of scene.changed.
type SceneInfo struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Width      int       `json:"width"`
	Height     int       `json:"height"`
	Grid       Grid      `json:"grid"`
	Background *AssetRef `json:"background"` // null: no background
	Fog        Fog       `json:"fog"`
}

// Info returns the scene without its tokens.
func (s SceneView) Info() SceneInfo {
	return SceneInfo{ID: s.ID, Name: s.Name, Width: s.Width, Height: s.Height, Grid: s.Grid, Background: s.Background, Fog: s.Fog}
}

// SceneListItem is a scene in the list of the GM.
type SceneListItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Sort int    `json:"sort"`
}

// SnapshotTable is the M1 part of SnapshotGame.
type SnapshotTable struct {
	ActiveSceneID *string         `json:"activeSceneId"` // null: no active scene
	Scenes        []SceneListItem `json:"scenes"`        // GM only; [] for players, never null
	Scene         *SceneView      `json:"scene"`         // the active scene rendered for the viewer; null: none
}

// SceneCreate is the data of scene.create.
type SceneCreate struct {
	Name   string `json:"name"`
	Width  *int   `json:"width,omitempty"`
	Height *int   `json:"height,omitempty"`
	Grid   *Grid  `json:"grid,omitempty"`
}

// SceneCreateAck is the data of the ack to scene.create.
type SceneCreateAck struct {
	SceneID string `json:"sceneId"`
}

// SceneUpdate is the data of scene.update: absent fields stay, and
// "backgroundAssetId": null removes the background. BackgroundAssetID is a
// json.RawMessage to tell an absent field from null.
type SceneUpdate struct {
	SceneID           string          `json:"sceneId"`
	Name              *string         `json:"name,omitempty"`
	Width             *int            `json:"width,omitempty"`
	Height            *int            `json:"height,omitempty"`
	Grid              *Grid           `json:"grid,omitempty"` // replaces the whole grid
	BackgroundAssetID json.RawMessage `json:"backgroundAssetId,omitempty"`
}

// SceneRef is the data of scene.delete, scene.activate and scene.view.
type SceneRef struct {
	SceneID string `json:"sceneId"`
}

// SceneReorder is the data of scene.reorder.
type SceneReorder struct {
	SceneIDs []string `json:"sceneIds"`
}

// ScenesChanged is the data of scenes.changed.
type ScenesChanged struct {
	Scenes []SceneListItem `json:"scenes"`
}

// SceneChanged is the data of scene.changed.
type SceneChanged struct {
	Scene SceneInfo `json:"scene"`
}

// SceneActivated is the data of scene.activated.
type SceneActivated struct {
	Scene SceneView `json:"scene"`
}

// SceneViewAck is the data of the ack to scene.view.
type SceneViewAck struct {
	Scene SceneView `json:"scene"`
}

// Message types of the tokens of M1.
const (
	TypeTokenCreate   = "token.create"
	TypeTokenUpdate   = "token.update"
	TypeTokenMove     = "token.move"
	TypeTokenDrag     = "token.drag"
	TypeTokenDelete   = "token.delete"
	TypeTokenChanged  = "token.changed"
	TypeTokenRemoved  = "token.removed"
	TypeTokenDragging = "token.dragging"
)

// TokenBar is a resource bar of a token (hit points and the like).
type TokenBar struct {
	Key        string `json:"key"`
	Value      int    `json:"value"`
	Max        int    `json:"max"`
	Visibility string `json:"visibility"` // all | owners | gm
}

// Token is a token as a client sees it.
type Token struct {
	ID             string     `json:"id"`
	SceneID        string     `json:"sceneId"`
	Layer          string     `json:"layer"` // map | tokens | gm
	X              float64    `json:"x"`
	Y              float64    `json:"y"`
	W              float64    `json:"w"`
	H              float64    `json:"h"`
	Rotation       float64    `json:"rotation"`
	Z              int        `json:"z"`
	Name           string     `json:"name"`
	NameVisibility string     `json:"nameVisibility"` // all | hover | gm
	Image          *AssetRef  `json:"image"`          // null: none
	Hidden         bool       `json:"hidden"`
	Locked         bool       `json:"locked"`
	Owners         []string   `json:"owners"`     // never null
	Bars           []TokenBar `json:"bars"`       // never null
	Conditions     []string   `json:"conditions"` // never null
}

// TokenFields are the settable fields; nil means "not given". ImageAssetID is
// raw to tell absent from null (null removes the image). Decoding tells an
// absent Owners, Bars or Conditions (nil) from an empty list (non-nil, clears).
type TokenFields struct {
	Layer          *string         `json:"layer,omitempty"`
	X              *float64        `json:"x,omitempty"`
	Y              *float64        `json:"y,omitempty"`
	W              *float64        `json:"w,omitempty"`
	H              *float64        `json:"h,omitempty"`
	Rotation       *float64        `json:"rotation,omitempty"`
	Z              *int            `json:"z,omitempty"`
	Name           *string         `json:"name,omitempty"`
	NameVisibility *string         `json:"nameVisibility,omitempty"`
	ImageAssetID   json.RawMessage `json:"imageAssetId,omitempty"`
	Hidden         *bool           `json:"hidden,omitempty"`
	Locked         *bool           `json:"locked,omitempty"`
	Owners         []string        `json:"owners,omitempty"`
	Bars           []TokenBar      `json:"bars,omitempty"`
	Conditions     []string        `json:"conditions,omitempty"`
}

// tokenFieldsWire is TokenFields for encoding: the lists are pointers so an
// empty list is written and only a nil one is left out.
type tokenFieldsWire struct {
	Layer          *string         `json:"layer,omitempty"`
	X              *float64        `json:"x,omitempty"`
	Y              *float64        `json:"y,omitempty"`
	W              *float64        `json:"w,omitempty"`
	H              *float64        `json:"h,omitempty"`
	Rotation       *float64        `json:"rotation,omitempty"`
	Z              *int            `json:"z,omitempty"`
	Name           *string         `json:"name,omitempty"`
	NameVisibility *string         `json:"nameVisibility,omitempty"`
	ImageAssetID   json.RawMessage `json:"imageAssetId,omitempty"`
	Hidden         *bool           `json:"hidden,omitempty"`
	Locked         *bool           `json:"locked,omitempty"`
	Owners         *[]string       `json:"owners,omitempty"`
	Bars           *[]TokenBar     `json:"bars,omitempty"`
	Conditions     *[]string       `json:"conditions,omitempty"`
}

func (f TokenFields) wire() tokenFieldsWire {
	w := tokenFieldsWire{
		Layer: f.Layer, X: f.X, Y: f.Y, W: f.W, H: f.H, Rotation: f.Rotation, Z: f.Z,
		Name: f.Name, NameVisibility: f.NameVisibility, ImageAssetID: f.ImageAssetID,
		Hidden: f.Hidden, Locked: f.Locked,
	}
	if f.Owners != nil {
		w.Owners = &f.Owners
	}
	if f.Bars != nil {
		w.Bars = &f.Bars
	}
	if f.Conditions != nil {
		w.Conditions = &f.Conditions
	}
	return w
}

// TokenCreate is the data of token.create.
type TokenCreate struct {
	SceneID string `json:"sceneId"`
	TokenFields
}

// MarshalJSON writes empty lists of the fields (TokenFields has no method of
// its own: it would be promoted and break decoding).
func (c TokenCreate) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		SceneID string `json:"sceneId"`
		tokenFieldsWire
	}{c.SceneID, c.wire()})
}

// TokenCreateAck is the data of the ack to token.create.
type TokenCreateAck struct {
	TokenID string `json:"tokenId"`
}

// TokenUpdate is the data of token.update.
type TokenUpdate struct {
	TokenID string `json:"tokenId"`
	TokenFields
}

// MarshalJSON is TokenCreate.MarshalJSON for token.update.
func (u TokenUpdate) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		TokenID string `json:"tokenId"`
		tokenFieldsWire
	}{u.TokenID, u.wire()})
}

// TokenMove is the data of token.move, token.drag and token.dragging.
type TokenMove struct {
	TokenID string  `json:"tokenId"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
}

// TokenRef is the data of token.delete and token.removed.
type TokenRef struct {
	TokenID string `json:"tokenId"`
}

// TokenChanged is the data of token.changed.
type TokenChanged struct {
	Token Token `json:"token"`
}
