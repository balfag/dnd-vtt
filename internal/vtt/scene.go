// Package vtt implements the table of M1: scenes now; tokens, fog and drawings join later.
package vtt

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/balfag/dnd-vtt/internal/id"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

func newID() string { return id.New() }

var (
	ErrNotFound = errors.New("vtt: not found")
	ErrInvalid  = errors.New("vtt: invalid data")
	ErrConflict = errors.New("vtt: conflict")
)

// Limits and defaults of a scene (data-model.md, «Scene»).
const (
	maxNameRunes  = 200
	minSceneSize  = 100
	maxSceneSize  = 20000
	defaultWidth  = 2800
	defaultHeight = 2100
	minGridSize   = 10
	maxGridSize   = 500
	minUnitFt     = 1
	maxUnitFt     = 100
)

type Options struct{ Now func() time.Time }

// Service is safe for concurrent use. Every change runs in one transaction.
type Service struct {
	db  *sql.DB
	now func() time.Time
}

func NewService(db *sql.DB, opts Options) *Service {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, now: now}
}

// DefaultGrid is the grid of a scene created without one.
func DefaultGrid() protocol.Grid {
	return protocol.Grid{Type: "square", SizePx: 70, UnitFt: 5, OffsetX: 0, OffsetY: 0, Color: "#9aa8b3", Visible: true}
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxNameRunes {
		return "", invalid("name must be 1..%d characters", maxNameRunes)
	}
	return name, nil
}

func validateSize(width, height int) error {
	if width < minSceneSize || width > maxSceneSize || height < minSceneSize || height > maxSceneSize {
		return invalid("width and height must be %d..%d", minSceneSize, maxSceneSize)
	}
	return nil
}

func validColor(c string) bool {
	if len(c) != 7 || c[0] != '#' {
		return false
	}
	for _, r := range c[1:] {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func validateGrid(g protocol.Grid) error {
	switch g.Type {
	case "square", "hexPointy", "hexFlat":
	default:
		return invalid("grid type must be square, hexPointy or hexFlat")
	}
	if g.SizePx < minGridSize || g.SizePx > maxGridSize {
		return invalid("grid sizePx must be %d..%d", minGridSize, maxGridSize)
	}
	if g.UnitFt < minUnitFt || g.UnitFt > maxUnitFt {
		return invalid("grid unitFt must be %d..%d", minUnitFt, maxUnitFt)
	}
	limit := float64(g.SizePx)
	if !(g.OffsetX >= 0 && g.OffsetX <= limit) || !(g.OffsetY >= 0 && g.OffsetY <= limit) {
		return invalid("grid offsets must be 0..sizePx")
	}
	if !validColor(g.Color) {
		return invalid("grid color must be #rrggbb in lower case")
	}
	return nil
}

func clamp(n, lo, hi int) int { return max(lo, min(n, hi)) }

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// sceneRow is a scene as stored.
type sceneRow struct {
	id         string
	name       string
	width      int
	height     int
	grid       protocol.Grid
	background sql.NullString
	fog        protocol.Fog
}

func (r sceneRow) view() protocol.SceneView {
	v := protocol.SceneView{ID: r.id, Name: r.name, Width: r.width, Height: r.height, Grid: r.grid, Fog: r.fog, Tokens: []protocol.Token{}}
	if r.background.Valid {
		v.Background = &protocol.AssetRef{AssetID: r.background.String, URL: "/assets/" + r.background.String}
	}
	return v
}

const sceneSelect = `SELECT id, name, width, height, grid, background_asset_id, fog FROM scene`

// loadScene reads a scene of the campaign; another campaign's scene is ErrNotFound.
func loadScene(ctx context.Context, q queryer, campaignID, sceneID string) (sceneRow, error) {
	var (
		r       sceneRow
		gridRaw string
		fogRaw  string
	)
	err := q.QueryRowContext(ctx, sceneSelect+` WHERE id = ? AND campaign_id = ?`, sceneID, campaignID).
		Scan(&r.id, &r.name, &r.width, &r.height, &gridRaw, &r.background, &fogRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return sceneRow{}, ErrNotFound
	}
	if err != nil {
		return sceneRow{}, fmt.Errorf("load scene: %w", err)
	}
	if err := json.Unmarshal([]byte(gridRaw), &r.grid); err != nil {
		return sceneRow{}, fmt.Errorf("decode grid of scene %s: %w", sceneID, err)
	}
	if err := json.Unmarshal([]byte(fogRaw), &r.fog); err != nil {
		return sceneRow{}, fmt.Errorf("decode fog of scene %s: %w", sceneID, err)
	}
	if r.fog.Ops == nil {
		r.fog.Ops = []protocol.FogOp{}
	}
	return r, nil
}

func listScenes(ctx context.Context, q queryer, campaignID string) ([]protocol.SceneListItem, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, name, sort FROM scene WHERE campaign_id = ? ORDER BY sort, id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list scenes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []protocol.SceneListItem{}
	for rows.Next() {
		var it protocol.SceneListItem
		if err := rows.Scan(&it.ID, &it.Name, &it.Sort); err != nil {
			return nil, fmt.Errorf("scan scene: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list scenes: %w", err)
	}
	return out, nil
}

// activeSceneID returns the active scene of the campaign; "" when there is none.
func activeSceneID(ctx context.Context, q queryer, campaignID string) (string, error) {
	var id sql.NullString
	err := q.QueryRowContext(ctx, `SELECT active_scene_id FROM campaign WHERE id = ?`, campaignID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read active scene: %w", err)
	}
	return id.String, nil
}

// tx runs fn in a transaction (BEGIN IMMEDIATE, set by the store).
func (s *Service) tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// CreateResult is the outcome of CreateScene.
type CreateResult struct {
	SceneID string
	Scenes  []protocol.SceneListItem
	// Activated is the new scene when it became the active one because the
	// campaign had none; nil otherwise.
	Activated *protocol.SceneView
}

// CreateScene adds a scene to the campaign. Without an active scene the new one
// becomes active.
func (s *Service) CreateScene(ctx context.Context, campaignID string, in protocol.SceneCreate) (CreateResult, error) {
	name, err := validateName(in.Name)
	if err != nil {
		return CreateResult{}, err
	}
	width, height := defaultWidth, defaultHeight
	if in.Width != nil {
		width = *in.Width
	}
	if in.Height != nil {
		height = *in.Height
	}
	if err := validateSize(width, height); err != nil {
		return CreateResult{}, err
	}
	grid := DefaultGrid()
	if in.Grid != nil {
		grid = *in.Grid
	}
	if err := validateGrid(grid); err != nil {
		return CreateResult{}, err
	}
	gridJSON, err := json.Marshal(grid)
	if err != nil {
		return CreateResult{}, fmt.Errorf("encode grid: %w", err)
	}

	var res CreateResult
	err = s.tx(ctx, func(tx *sql.Tx) error {
		active, err := activeSceneID(ctx, tx, campaignID)
		if err != nil {
			return err
		}
		var sort int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort) + 1, 0) FROM scene WHERE campaign_id = ?`, campaignID).Scan(&sort); err != nil {
			return fmt.Errorf("next sort: %w", err)
		}
		res.SceneID = newID()
		nowMs := s.now().UnixMilli()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO scene (id, campaign_id, name, sort, width, height, grid, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			res.SceneID, campaignID, name, sort, width, height, string(gridJSON), nowMs, nowMs); err != nil {
			return fmt.Errorf("insert scene: %w", err)
		}
		if active == "" {
			if _, err := tx.ExecContext(ctx, `UPDATE campaign SET active_scene_id = ? WHERE id = ?`, res.SceneID, campaignID); err != nil {
				return fmt.Errorf("activate the first scene: %w", err)
			}
			row, err := loadScene(ctx, tx, campaignID, res.SceneID)
			if err != nil {
				return err
			}
			view := row.view()
			res.Activated = &view
		}
		res.Scenes, err = listScenes(ctx, tx, campaignID)
		return err
	})
	return res, err
}

// UpdateResult is the outcome of UpdateScene.
type UpdateResult struct {
	Scene protocol.SceneView
	// WasActive tells that the scene was the active one when it changed.
	WasActive bool
}

// UpdateScene changes the given fields of a scene. A background of another
// campaign is ErrNotFound; "backgroundAssetId": null removes the background;
// a new background without sizes in the same command sets them from the image.
func (s *Service) UpdateScene(ctx context.Context, campaignID string, in protocol.SceneUpdate) (UpdateResult, error) {
	// Check what needs no database first.
	var name *string
	if in.Name != nil {
		n, err := validateName(*in.Name)
		if err != nil {
			return UpdateResult{}, err
		}
		name = &n
	}
	if in.Width != nil && (*in.Width < minSceneSize || *in.Width > maxSceneSize) ||
		in.Height != nil && (*in.Height < minSceneSize || *in.Height > maxSceneSize) {
		return UpdateResult{}, invalid("width and height must be %d..%d", minSceneSize, maxSceneSize)
	}
	if in.Grid != nil {
		if err := validateGrid(*in.Grid); err != nil {
			return UpdateResult{}, err
		}
	}
	setBackground, newBackground, err := parseBackground(in.BackgroundAssetID)
	if err != nil {
		return UpdateResult{}, err
	}

	var res UpdateResult
	err = s.tx(ctx, func(tx *sql.Tx) error {
		row, err := loadScene(ctx, tx, campaignID, in.SceneID)
		if err != nil {
			return err
		}
		if name != nil {
			row.name = *name
		}
		if in.Width != nil {
			row.width = *in.Width
		}
		if in.Height != nil {
			row.height = *in.Height
		}
		if in.Grid != nil {
			row.grid = *in.Grid
		}
		if setBackground {
			row.background = sql.NullString{}
			if newBackground != "" {
				var w, h int
				err := tx.QueryRowContext(ctx, `SELECT width, height FROM asset WHERE id = ? AND campaign_id = ?`, newBackground, campaignID).Scan(&w, &h)
				if errors.Is(err, sql.ErrNoRows) {
					return ErrNotFound
				}
				if err != nil {
					return fmt.Errorf("read background asset: %w", err)
				}
				row.background = sql.NullString{String: newBackground, Valid: true}
				if in.Width == nil {
					row.width = clamp(w, minSceneSize, maxSceneSize)
				}
				if in.Height == nil {
					row.height = clamp(h, minSceneSize, maxSceneSize)
				}
			}
		}

		gridJSON, err := json.Marshal(row.grid)
		if err != nil {
			return fmt.Errorf("encode grid: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE scene SET name = ?, width = ?, height = ?, grid = ?, background_asset_id = ?, updated_at = ? WHERE id = ?`,
			row.name, row.width, row.height, string(gridJSON), row.background, s.now().UnixMilli(), row.id); err != nil {
			return fmt.Errorf("update scene: %w", err)
		}
		active, err := activeSceneID(ctx, tx, campaignID)
		if err != nil {
			return err
		}
		res = UpdateResult{Scene: row.view(), WasActive: active == row.id}
		return nil
	})
	return res, err
}

// parseBackground reads backgroundAssetId: absent leaves it (set=false), null
// removes it (set=true, id=""), a string sets it.
func parseBackground(raw json.RawMessage) (set bool, id string, err error) {
	if len(raw) == 0 {
		return false, "", nil
	}
	if string(raw) == "null" {
		return true, "", nil
	}
	if err := json.Unmarshal(raw, &id); err != nil || id == "" {
		return false, "", invalid("backgroundAssetId must be a string or null")
	}
	return true, id, nil
}

// DeleteScene removes a scene with its tokens and drawings. The active scene
// cannot be deleted (ErrConflict). It returns the list that remains.
func (s *Service) DeleteScene(ctx context.Context, campaignID, sceneID string) ([]protocol.SceneListItem, error) {
	var out []protocol.SceneListItem
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := loadScene(ctx, tx, campaignID, sceneID); err != nil {
			return err
		}
		active, err := activeSceneID(ctx, tx, campaignID)
		if err != nil {
			return err
		}
		if active == sceneID {
			return fmt.Errorf("%w: the active scene cannot be deleted", ErrConflict)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM scene WHERE id = ?`, sceneID); err != nil {
			return fmt.Errorf("delete scene: %w", err)
		}
		out, err = listScenes(ctx, tx, campaignID)
		return err
	})
	return out, err
}

// ReorderScenes sets the order of the campaign's scenes: ids must be exactly
// the set of its scenes, without repeats.
func (s *Service) ReorderScenes(ctx context.Context, campaignID string, ids []string) ([]protocol.SceneListItem, error) {
	var out []protocol.SceneListItem
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := activeSceneID(ctx, tx, campaignID); err != nil { // the campaign exists
			return err
		}
		current, err := listScenes(ctx, tx, campaignID)
		if err != nil {
			return err
		}
		if len(ids) != len(current) {
			return invalid("sceneIds must list every scene of the campaign once")
		}
		known := make(map[string]bool, len(current))
		for _, it := range current {
			known[it.ID] = true
		}
		for _, id := range ids {
			if !known[id] {
				return invalid("sceneIds must list every scene of the campaign once")
			}
			delete(known, id) // a repeat is not known any more
		}
		nowMs := s.now().UnixMilli()
		for i, id := range ids {
			if _, err := tx.ExecContext(ctx, `UPDATE scene SET sort = ?, updated_at = ? WHERE id = ?`, i, nowMs, id); err != nil {
				return fmt.Errorf("reorder scenes: %w", err)
			}
		}
		out, err = listScenes(ctx, tx, campaignID)
		return err
	})
	return out, err
}

// ActivateScene makes the scene the active one of the campaign. The view holds
// every token of the scene; render it per viewer with sceneFor.
func (s *Service) ActivateScene(ctx context.Context, campaignID, sceneID string) (protocol.SceneView, error) {
	var view protocol.SceneView
	err := s.tx(ctx, func(tx *sql.Tx) error {
		row, err := loadScene(ctx, tx, campaignID, sceneID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE campaign SET active_scene_id = ? WHERE id = ?`, sceneID, campaignID); err != nil {
			return fmt.Errorf("activate scene: %w", err)
		}
		view = row.view()
		view.Tokens, err = listTokens(ctx, tx, sceneID)
		return err
	})
	return view, err
}

// ViewScene reads a scene of the campaign with all its tokens, for the GM.
func (s *Service) ViewScene(ctx context.Context, campaignID, sceneID string) (protocol.SceneView, error) {
	row, err := loadScene(ctx, s.db, campaignID, sceneID)
	if err != nil {
		return protocol.SceneView{}, err
	}
	view := row.view()
	view.Tokens, err = listTokens(ctx, s.db, sceneID)
	return view, err
}

// BackgroundOfActiveScene reports whether assetID is the background of the
// active scene of campaignID (for GET /assets/{id}).
func (s *Service) BackgroundOfActiveScene(ctx context.Context, campaignID, assetID string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM campaign c JOIN scene s ON s.id = c.active_scene_id WHERE c.id = ? AND s.background_asset_id = ?`,
		campaignID, assetID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check the background: %w", err)
	}
	return true, nil
}
