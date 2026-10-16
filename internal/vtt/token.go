package vtt

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// Limits and defaults of a token (data-model.md, «Token»).
const (
	maxTokenNameRunes = 100
	minTokenSize      = 0.25
	maxTokenSize      = 20
	maxOwners         = 20
	maxBars           = 5
	maxConditions     = 20
	minBarValue       = -9999
	maxBarValue       = 99999
)

var (
	barKeyPattern    = regexp.MustCompile(`^[a-z0-9_]{1,16}$`)
	conditionPattern = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)
)

// TokenResult is a token after a change with the state of its scene at that
// moment; the renderer of the event captures both.
type TokenResult struct {
	Token protocol.Token
	State sceneState
}

// tokenScene is the scene a token is on, as far as token checks need it.
type tokenScene struct {
	id     string
	width  int
	height int
	active bool
	fog    protocol.Fog
}

func (sc tokenScene) state() sceneState { return sceneState{Active: sc.active, Fog: sc.fog} }

const rowSelect = `SELECT t.id, t.scene_id, t.layer, t.x, t.y, t.w, t.h, t.rotation, t.z, t.name, t.name_visibility,
	t.image_asset_id, t.hidden, t.locked, t.owners, t.bars, t.conditions FROM token t`

// rowScanner is *sql.Row and *sql.Rows.
type rowScanner interface{ Scan(dest ...any) error }

func scanToken(r rowScanner) (protocol.Token, error) {
	var (
		t                           protocol.Token
		image                       sql.NullString
		hidden, locked              int
		ownersRaw, barsRaw, condRaw string
	)
	if err := r.Scan(&t.ID, &t.SceneID, &t.Layer, &t.X, &t.Y, &t.W, &t.H, &t.Rotation, &t.Z, &t.Name,
		&t.NameVisibility, &image, &hidden, &locked, &ownersRaw, &barsRaw, &condRaw); err != nil {
		return protocol.Token{}, err
	}
	t.Hidden, t.Locked = hidden != 0, locked != 0
	if image.Valid {
		t.Image = &protocol.AssetRef{AssetID: image.String, URL: "/assets/" + image.String}
	}
	t.Owners, t.Bars, t.Conditions = []string{}, []protocol.TokenBar{}, []string{}
	for _, f := range []struct {
		name string
		raw  string
		dst  any
	}{{"owners", ownersRaw, &t.Owners}, {"bars", barsRaw, &t.Bars}, {"conditions", condRaw, &t.Conditions}} {
		if err := json.Unmarshal([]byte(f.raw), f.dst); err != nil {
			return protocol.Token{}, fmt.Errorf("decode %s of token %s: %w", f.name, t.ID, err)
		}
	}
	if t.Owners == nil {
		t.Owners = []string{}
	}
	if t.Bars == nil {
		t.Bars = []protocol.TokenBar{}
	}
	if t.Conditions == nil {
		t.Conditions = []string{}
	}
	return t, nil
}

// listTokens reads every token of a scene, by z then id.
func listTokens(ctx context.Context, q queryer, sceneID string) ([]protocol.Token, error) {
	rows, err := q.QueryContext(ctx, rowSelect+` WHERE t.scene_id = ? ORDER BY t.z, t.id`, sceneID)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []protocol.Token{}
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	return out, nil
}

// loadToken reads a token of the campaign with its scene; another campaign's
// token is ErrNotFound.
func loadToken(ctx context.Context, q queryer, campaignID, tokenID string) (protocol.Token, tokenScene, error) {
	t, err := scanToken(q.QueryRowContext(ctx, rowSelect+` JOIN scene s ON s.id = t.scene_id WHERE t.id = ? AND s.campaign_id = ?`, tokenID, campaignID))
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.Token{}, tokenScene{}, ErrNotFound
	}
	if err != nil {
		return protocol.Token{}, tokenScene{}, fmt.Errorf("load token: %w", err)
	}
	sc, err := loadTokenScene(ctx, q, campaignID, t.SceneID)
	return t, sc, err
}

func loadTokenScene(ctx context.Context, q queryer, campaignID, sceneID string) (tokenScene, error) {
	row, err := loadScene(ctx, q, campaignID, sceneID)
	if err != nil {
		return tokenScene{}, err
	}
	active, err := activeSceneID(ctx, q, campaignID)
	if err != nil {
		return tokenScene{}, err
	}
	return tokenScene{id: row.id, width: row.width, height: row.height, active: active == row.id, fog: row.fog}, nil
}

func finite(ns ...float64) bool {
	for _, n := range ns {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return false
		}
	}
	return true
}

// normalizeRotation maps any finite angle to [0, 360).
func normalizeRotation(r float64) float64 {
	r = math.Mod(r, 360)
	if r < 0 {
		r += 360
	}
	if r >= 360 { // a tiny negative angle rounds up to exactly 360
		r = 0
	}
	return r
}

func validEnum(v string, allowed ...string) bool { return slices.Contains(allowed, v) }

func validateBars(bars []protocol.TokenBar) error {
	if len(bars) > maxBars {
		return invalid("a token has at most %d bars", maxBars)
	}
	seen := map[string]bool{}
	for _, b := range bars {
		switch {
		case !barKeyPattern.MatchString(b.Key):
			return invalid("bar key must match [a-z0-9_]{1,16}")
		case seen[b.Key]:
			return invalid("bar key %q is repeated", b.Key)
		case b.Value < minBarValue || b.Value > maxBarValue || b.Max < minBarValue || b.Max > maxBarValue:
			return invalid("bar value and max must be %d..%d", minBarValue, maxBarValue)
		case b.Max < 1:
			return invalid("bar max must be at least 1")
		case !validEnum(b.Visibility, "all", "owners", "gm"):
			return invalid("bar visibility must be all, owners or gm")
		}
		seen[b.Key] = true
	}
	return nil
}

func validateConditions(list []string) error {
	if len(list) > maxConditions {
		return invalid("a token has at most %d conditions", maxConditions)
	}
	seen := map[string]bool{}
	for _, c := range list {
		if !conditionPattern.MatchString(c) {
			return invalid("condition must match [a-z0-9_-]{1,32}")
		}
		if seen[c] {
			return invalid("condition %q is repeated", c)
		}
		seen[c] = true
	}
	return nil
}

// validateOwners checks that every owner is a participant of the campaign who
// is not removed.
func validateOwners(ctx context.Context, q queryer, campaignID string, owners []string) error {
	if len(owners) > maxOwners {
		return invalid("a token has at most %d owners", maxOwners)
	}
	seen := map[string]bool{}
	for _, o := range owners {
		if seen[o] {
			return invalid("owner %q is repeated", o)
		}
		seen[o] = true
		var one int
		err := q.QueryRowContext(ctx, `SELECT 1 FROM player WHERE id = ? AND campaign_id = ? AND removed = 0`, o, campaignID).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("check owner: %w", err)
		}
	}
	return nil
}

// parseImage reads imageAssetId like parseBackground: absent leaves it, null removes it.
func parseImage(raw json.RawMessage) (set bool, id string, err error) {
	if len(raw) == 0 {
		return false, "", nil
	}
	if string(raw) == "null" {
		return true, "", nil
	}
	if err := json.Unmarshal(raw, &id); err != nil || id == "" {
		return false, "", invalid("imageAssetId must be a string or null")
	}
	return true, id, nil
}

// applyFields checks the given fields and sets them on t. Only given fields
// are checked; a token that is out of bounds after the scene shrank can still
// be renamed.
func applyFields(ctx context.Context, q queryer, campaignID string, sc tokenScene, t *protocol.Token, f protocol.TokenFields) error {
	nums := []*float64{f.X, f.Y, f.W, f.H, f.Rotation}
	for _, n := range nums {
		if n != nil && !finite(*n) {
			return invalid("numbers must be finite")
		}
	}
	if f.Layer != nil {
		if !validEnum(*f.Layer, "map", "tokens", "gm") {
			return invalid("layer must be map, tokens or gm")
		}
		t.Layer = *f.Layer
	}
	if f.X != nil {
		if *f.X < 0 || *f.X > float64(sc.width) {
			return invalid("x must be 0..%d", sc.width)
		}
		t.X = *f.X
	}
	if f.Y != nil {
		if *f.Y < 0 || *f.Y > float64(sc.height) {
			return invalid("y must be 0..%d", sc.height)
		}
		t.Y = *f.Y
	}
	if f.W != nil {
		if *f.W < minTokenSize || *f.W > maxTokenSize {
			return invalid("w must be %v..%v", minTokenSize, maxTokenSize)
		}
		t.W = *f.W
	}
	if f.H != nil {
		if *f.H < minTokenSize || *f.H > maxTokenSize {
			return invalid("h must be %v..%v", minTokenSize, maxTokenSize)
		}
		t.H = *f.H
	}
	if f.Rotation != nil {
		t.Rotation = normalizeRotation(*f.Rotation)
	}
	if f.Z != nil {
		t.Z = *f.Z
	}
	if f.Name != nil {
		name := strings.TrimSpace(*f.Name)
		if utf8.RuneCountInString(name) > maxTokenNameRunes {
			return invalid("name must be at most %d characters", maxTokenNameRunes)
		}
		t.Name = name
	}
	if f.NameVisibility != nil {
		if !validEnum(*f.NameVisibility, "all", "hover", "gm") {
			return invalid("nameVisibility must be all, hover or gm")
		}
		t.NameVisibility = *f.NameVisibility
	}
	if err := applyImage(ctx, q, campaignID, t, f.ImageAssetID); err != nil {
		return err
	}
	if f.Hidden != nil {
		t.Hidden = *f.Hidden
	}
	if f.Locked != nil {
		t.Locked = *f.Locked
	}
	if f.Owners != nil {
		if err := validateOwners(ctx, q, campaignID, f.Owners); err != nil {
			return err
		}
		t.Owners = slices.Clone(f.Owners)
	}
	if f.Bars != nil {
		if err := validateBars(f.Bars); err != nil {
			return err
		}
		t.Bars = slices.Clone(f.Bars)
	}
	if f.Conditions != nil {
		if err := validateConditions(f.Conditions); err != nil {
			return err
		}
		t.Conditions = slices.Clone(f.Conditions)
	}
	return nil
}

func applyImage(ctx context.Context, q queryer, campaignID string, t *protocol.Token, raw json.RawMessage) error {
	set, id, err := parseImage(raw)
	if err != nil || !set {
		return err
	}
	if id == "" {
		t.Image = nil
		return nil
	}
	var one int
	err = q.QueryRowContext(ctx, `SELECT 1 FROM asset WHERE id = ? AND campaign_id = ?`, id, campaignID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read token image: %w", err)
	}
	t.Image = &protocol.AssetRef{AssetID: id, URL: "/assets/" + id}
	return nil
}

// encodeLists encodes owners, bars and conditions for the token table.
func encodeLists(t protocol.Token) (owners, bars, conditions string, err error) {
	var parts [3][]byte
	for i, v := range []any{t.Owners, t.Bars, t.Conditions} {
		if parts[i], err = json.Marshal(v); err != nil {
			return "", "", "", fmt.Errorf("encode token lists: %w", err)
		}
	}
	return string(parts[0]), string(parts[1]), string(parts[2]), nil
}

func imageID(t protocol.Token) sql.NullString {
	if t.Image == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: t.Image.AssetID, Valid: true}
}

// CreateToken adds a token to a scene of the campaign. x and y are required.
func (s *Service) CreateToken(ctx context.Context, campaignID string, in protocol.TokenCreate) (TokenResult, error) {
	if in.X == nil || in.Y == nil {
		return TokenResult{}, invalid("x and y are required")
	}
	var res TokenResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		sc, err := loadTokenScene(ctx, tx, campaignID, in.SceneID)
		if err != nil {
			return err
		}
		t := protocol.Token{
			ID: newID(), SceneID: sc.id, Layer: "tokens", W: 1, H: 1, NameVisibility: "hover",
			Owners: []string{}, Bars: []protocol.TokenBar{}, Conditions: []string{},
		}
		if in.Z == nil {
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(z) + 1, 0) FROM token WHERE scene_id = ?`, sc.id).Scan(&t.Z); err != nil {
				return fmt.Errorf("next z: %w", err)
			}
		}
		if err := applyFields(ctx, tx, campaignID, sc, &t, in.TokenFields); err != nil {
			return err
		}
		owners, bars, conditions, err := encodeLists(t)
		if err != nil {
			return err
		}
		nowMs := s.now().UnixMilli()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO token (id, scene_id, layer, x, y, w, h, rotation, z, name, name_visibility, image_asset_id, hidden, locked, owners, bars, conditions, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			t.ID, t.SceneID, t.Layer, t.X, t.Y, t.W, t.H, t.Rotation, t.Z, t.Name, t.NameVisibility, imageID(t),
			t.Hidden, t.Locked, owners, bars, conditions, nowMs, nowMs); err != nil {
			return fmt.Errorf("insert token: %w", err)
		}
		res = TokenResult{Token: t, State: sc.state()}
		return nil
	})
	return res, err
}

func (s *Service) saveToken(ctx context.Context, tx *sql.Tx, t protocol.Token) error {
	owners, bars, conditions, err := encodeLists(t)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE token SET layer = ?, x = ?, y = ?, w = ?, h = ?, rotation = ?, z = ?, name = ?, name_visibility = ?,
		 image_asset_id = ?, hidden = ?, locked = ?, owners = ?, bars = ?, conditions = ?, updated_at = ? WHERE id = ?`,
		t.Layer, t.X, t.Y, t.W, t.H, t.Rotation, t.Z, t.Name, t.NameVisibility, imageID(t),
		t.Hidden, t.Locked, owners, bars, conditions, s.now().UnixMilli(), t.ID); err != nil {
		return fmt.Errorf("update token: %w", err)
	}
	return nil
}

// UpdateToken changes the given fields of a token of the campaign.
func (s *Service) UpdateToken(ctx context.Context, campaignID string, in protocol.TokenUpdate) (TokenResult, error) {
	var res TokenResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		t, sc, err := loadToken(ctx, tx, campaignID, in.TokenID)
		if err != nil {
			return err
		}
		if err := applyFields(ctx, tx, campaignID, sc, &t, in.TokenFields); err != nil {
			return err
		}
		if err := s.saveToken(ctx, tx, t); err != nil {
			return err
		}
		res = TokenResult{Token: t, State: sc.state()}
		return nil
	})
	return res, err
}

// MoveToken sets the position of a token. mayMove decides, from the token and
// its scene state, whether the caller may move it; its error is returned as is.
func (s *Service) MoveToken(ctx context.Context, campaignID string, in protocol.TokenMove, mayMove func(protocol.Token, sceneState) error) (TokenResult, error) {
	var res TokenResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		t, sc, err := loadToken(ctx, tx, campaignID, in.TokenID)
		if err != nil {
			return err
		}
		state := sc.state()
		if err := mayMove(t, state); err != nil {
			return err
		}
		if err := applyFields(ctx, tx, campaignID, sc, &t, protocol.TokenFields{X: &in.X, Y: &in.Y}); err != nil {
			return err
		}
		if err := s.saveToken(ctx, tx, t); err != nil {
			return err
		}
		res = TokenResult{Token: t, State: state}
		return nil
	})
	return res, err
}

// CheckDrag reads a token for token.drag without writing anything. ok is false
// when the token or the position is not acceptable or mayMove refuses.
func (s *Service) CheckDrag(ctx context.Context, campaignID string, in protocol.TokenMove, mayMove func(protocol.Token, sceneState) error) (TokenResult, bool, error) {
	t, sc, err := loadToken(ctx, s.db, campaignID, in.TokenID)
	if errors.Is(err, ErrNotFound) {
		return TokenResult{}, false, nil
	}
	if err != nil {
		return TokenResult{}, false, err
	}
	state := sc.state()
	if mayMove(t, state) != nil || !finite(in.X, in.Y) ||
		in.X < 0 || in.X > float64(sc.width) || in.Y < 0 || in.Y > float64(sc.height) {
		return TokenResult{}, false, nil
	}
	return TokenResult{Token: t, State: state}, true, nil
}

// DeleteToken removes a token of the campaign.
func (s *Service) DeleteToken(ctx context.Context, campaignID, tokenID string) (TokenResult, error) {
	var res TokenResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		t, sc, err := loadToken(ctx, tx, campaignID, tokenID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM token WHERE id = ?`, tokenID); err != nil {
			return fmt.Errorf("delete token: %w", err)
		}
		res = TokenResult{Token: t, State: sc.state()}
		return nil
	})
	return res, err
}

// GetToken reads a token of the campaign (for tests and tools).
func (s *Service) GetToken(ctx context.Context, campaignID, tokenID string) (protocol.Token, error) {
	t, _, err := loadToken(ctx, s.db, campaignID, tokenID)
	return t, err
}

// PlayerCanSeeTokenImage reports whether assetID is the image of a token of the
// active scene of the campaign that the player sees (for GET /assets/{id}).
func (s *Service) PlayerCanSeeTokenImage(ctx context.Context, campaignID, playerID, assetID string) (bool, error) {
	active, err := activeSceneID(ctx, s.db, campaignID)
	if err != nil || active == "" {
		return false, err
	}
	scene, err := loadScene(ctx, s.db, campaignID, active)
	if err != nil {
		return false, err
	}
	tokens, err := listTokens(ctx, s.db, active)
	if err != nil {
		return false, err
	}
	v := ws.Viewer{PlayerID: playerID}
	for _, t := range tokens {
		if t.Image != nil && t.Image.AssetID == assetID && TokenVisible(t, v, sceneState{Active: true, Fog: scene.fog}) {
			return true, nil
		}
	}
	return false, nil
}
