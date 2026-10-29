// Package protocol defines the WebSocket messages of docs/architecture/protocol.md.
package protocol

import (
	"encoding/json"

	"github.com/balfag/dnd-vtt/internal/dice"
)

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

// Fog is the fog of a scene. Ops are in the order of application.
type Fog struct {
	Enabled bool    `json:"enabled"`
	Ops     []FogOp `json:"ops"` // never null: "[]" for a new scene
}

// MarshalJSON encodes a nil Ops as [].
func (f Fog) MarshalJSON() ([]byte, error) {
	if f.Ops == nil {
		f.Ops = []FogOp{}
	}
	type plain Fog
	return json.Marshal(plain(f))
}

// FogOp reveals or hides the area of a shape.
type FogOp struct {
	Op    string `json:"op"` // reveal | hide
	Shape Shape  `json:"shape"`
}

// SceneView is a scene as a client sees it.
type SceneView struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Width      int       `json:"width"`
	Height     int       `json:"height"`
	Grid       Grid      `json:"grid"`
	Background *AssetRef `json:"background"` // null: no background
	Fog        Fog       `json:"fog"`
	Tokens     []Token   `json:"tokens"`   // visible to the viewer, by z then id; never null
	Drawings   []Drawing `json:"drawings"` // all of the scene, by id; never null
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
	Chat          []ChatMessage   `json:"chat"`          // last 50 visible to the viewer, ascending; never null
	Combat        *Combat         `json:"combat"`        // the tracker of the active scene for the viewer; null: none
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

// Message types of the fog of war of M1.
const (
	TypeFogSet     = "fog.set"
	TypeFogReveal  = "fog.reveal"
	TypeFogHide    = "fog.hide"
	TypeFogReset   = "fog.reset"
	TypeFogChanged = "fog.changed"
)

// FogSet is the data of fog.set.
type FogSet struct {
	SceneID string `json:"sceneId"`
	Enabled bool   `json:"enabled"`
}

// FogShape is the data of fog.reveal and fog.hide. fog.reset uses SceneRef.
type FogShape struct {
	SceneID string `json:"sceneId"`
	Shape   Shape  `json:"shape"`
}

// FogChanged is the data of fog.changed. Tokens is for players only: all the
// tokens of the scene they see now. nil is absent (the GM); a non-nil empty
// slice is encoded as [].
type FogChanged struct {
	SceneID string  `json:"sceneId"`
	Fog     Fog     `json:"fog"`
	Tokens  []Token `json:"tokens,omitempty"`
}

// MarshalJSON encodes Tokens when it is not nil, even if it is empty.
func (c FogChanged) MarshalJSON() ([]byte, error) {
	if c.Tokens == nil {
		return json.Marshal(struct {
			SceneID string `json:"sceneId"`
			Fog     Fog    `json:"fog"`
		}{c.SceneID, c.Fog})
	}
	return json.Marshal(struct {
		SceneID string  `json:"sceneId"`
		Fog     Fog     `json:"fog"`
		Tokens  []Token `json:"tokens"`
	}{c.SceneID, c.Fog, c.Tokens})
}

// Message types of the drawings, pings and rulers of M1.
const (
	TypeDrawingCreate  = "drawing.create"
	TypeDrawingUpdate  = "drawing.update"
	TypeDrawingDelete  = "drawing.delete"
	TypeDrawingChanged = "drawing.changed"
	TypeDrawingRemoved = "drawing.removed"
	TypePingPlace      = "ping.place"
	TypePingShown      = "ping.shown"
	TypeRulerUpdate    = "ruler.update"
	TypeRulerClear     = "ruler.clear"
	TypeRulerShown     = "ruler.shown"
)

// Drawing is a drawing or an area template of a scene.
type Drawing struct {
	ID             string          `json:"id"`
	SceneID        string          `json:"sceneId"`
	Kind           string          `json:"kind"` // freehand | rect | circle | text | template
	Data           json.RawMessage `json:"data"` // validated, stored and sent as given
	Color          string          `json:"color"`
	AuthorPlayerID string          `json:"authorPlayerId"`
}

// DrawingCreate is the data of drawing.create.
type DrawingCreate struct {
	SceneID string          `json:"sceneId"`
	Kind    string          `json:"kind"`
	Data    json.RawMessage `json:"data"`
	Color   string          `json:"color"`
}

// DrawingCreateAck is the data of the ack to drawing.create.
type DrawingCreateAck struct {
	DrawingID string `json:"drawingId"`
}

// DrawingUpdate is the data of drawing.update; absent fields stay.
type DrawingUpdate struct {
	DrawingID string          `json:"drawingId"`
	Data      json.RawMessage `json:"data,omitempty"`
	Color     *string         `json:"color,omitempty"`
}

// DrawingRef is the data of drawing.delete and drawing.removed.
type DrawingRef struct {
	DrawingID string `json:"drawingId"`
}

// DrawingChanged is the data of drawing.changed.
type DrawingChanged struct {
	Drawing Drawing `json:"drawing"`
}

// PingPlace is the data of ping.place; Focus (GM only) moves the cameras.
type PingPlace struct {
	SceneID string  `json:"sceneId"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Focus   bool    `json:"focus,omitempty"`
}

// PingShown is the data of ping.shown (no seq).
type PingShown struct {
	PlayerID string  `json:"playerId"`
	SceneID  string  `json:"sceneId"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Focus    bool    `json:"focus"`
}

// RulerUpdate is the data of ruler.update. ruler.clear uses SceneRef.
type RulerUpdate struct {
	SceneID string       `json:"sceneId"`
	Points  [][2]float64 `json:"points"`
}

// RulerShown is the data of ruler.shown (no seq).
type RulerShown struct {
	PlayerID string       `json:"playerId"`
	SceneID  string       `json:"sceneId"`
	Points   [][2]float64 `json:"points"` // [] when cleared, never null
}

// Message types of the chat of M1.
const (
	TypeChatSend    = "chat.send"
	TypeRollRequest = "roll.request"
	TypeChatHistory = "chat.history"
	TypeChatMessage = "chat.message"
)

// ChatSend is the data of chat.send; To makes the message private.
type ChatSend struct {
	Text string   `json:"text"`
	To   []string `json:"to,omitempty"`
}

// RollRequest is the data of roll.request.
type RollRequest struct {
	Formula    string `json:"formula"`
	Visibility string `json:"visibility"` // public | gm | self | blind
	Label      string `json:"label,omitempty"`
}

// ChatHistory is the data of chat.history.
type ChatHistory struct {
	Before string `json:"before"`
	Limit  int    `json:"limit,omitempty"` // 0 means 50
}

// ChatHistoryAck is the data of the ack to chat.history.
type ChatHistoryAck struct {
	Messages []ChatMessage `json:"messages"` // ascending, never null
}

// ChatRoll is the payload of a roll message.
type ChatRoll struct {
	Formula string       `json:"formula"` // canonical
	Label   string       `json:"label"`
	Result  *dice.Result `json:"result,omitempty"` // nil for the author of a blind roll
}

// ChatMessage is the data of chat.message and an item of the snapshot and history.
type ChatMessage struct {
	ID             string    `json:"id"`
	CreatedAt      int64     `json:"createdAt"`
	AuthorPlayerID *string   `json:"authorPlayerId"` // null for system messages
	Kind           string    `json:"kind"`
	Visibility     string    `json:"visibility"`
	To             []string  `json:"to,omitempty"`   // private only
	Text           string    `json:"text,omitempty"` // text and system
	Roll           *ChatRoll `json:"roll,omitempty"` // roll only

	Handout *ChatHandout `json:"handout,omitempty"` // handout only
}

// TypeHandoutShow shows a fragment of a wiki page to the players (GM only).
const TypeHandoutShow = "handout.show"

// HandoutShow is the data of handout.show; To makes the handout private.
type HandoutShow struct {
	Title   string          `json:"title"`
	Content json.RawMessage `json:"content"`
	To      []string        `json:"to,omitempty"`
}

// ChatHandout is the payload of a handout message.
type ChatHandout struct {
	Title   string          `json:"title"`
	Content json.RawMessage `json:"content"` // normalized wiki document fragment
}

// Message types of the initiative tracker of M1.
const (
	TypeCombatStart          = "combat.start"
	TypeCombatEnd            = "combat.end"
	TypeCombatAdd            = "combat.add"
	TypeCombatRemove         = "combat.remove"
	TypeCombatSetInitiative  = "combat.setInitiative"
	TypeCombatRollInitiative = "combat.rollInitiative"
	TypeCombatSetHidden      = "combat.setHidden"
	TypeCombatNext           = "combat.next"
	TypeCombatPrev           = "combat.prev"
	TypeCombatChanged        = "combat.changed"
)

// Combatant is a participant of the tracker, as a viewer sees it.
type Combatant struct {
	ID         string `json:"id"`
	TokenID    string `json:"tokenId"`
	Name       string `json:"name"`
	Initiative *int   `json:"initiative"` // null: not set
	Hidden     bool   `json:"hidden"`
}

// Combat is the tracker of a scene.
type Combat struct {
	SceneID         string      `json:"sceneId"`
	Round           int         `json:"round"`
	TurnCombatantID *string     `json:"turnCombatantId"` // null: before the first combat.next, or hidden from the viewer
	Combatants      []Combatant `json:"combatants"`      // turn order, never null
}

// CombatChanged is the data of combat.changed.
type CombatChanged struct {
	SceneID string  `json:"sceneId"`
	Combat  *Combat `json:"combat"` // null: the fight is over
}

// CombatAdd is the data of combat.add. combat.start, end, next and prev use SceneRef.
type CombatAdd struct {
	SceneID  string   `json:"sceneId"`
	TokenIDs []string `json:"tokenIds"`
}

// CombatantRef is the data of combat.remove.
type CombatantRef struct {
	CombatantID string `json:"combatantId"`
}

// CombatSetInitiative is the data of combat.setInitiative. Value is required:
// a null clears the initiative, an absent field is bad_request, so handlers
// check it in the raw data.
type CombatSetInitiative struct {
	CombatantID string `json:"combatantId"`
	Value       *int   `json:"value"` // null clears; the field is required
}

// CombatRollInitiative is the data of combat.rollInitiative.
type CombatRollInitiative struct {
	CombatantIDs []string `json:"combatantIds"`
}

// CombatSetHidden is the data of combat.setHidden.
type CombatSetHidden struct {
	CombatantID string `json:"combatantId"`
	Hidden      bool   `json:"hidden"`
}
