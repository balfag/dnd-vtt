package vtt

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// Limits of the drawings (protocol.md, «Рисунки и шаблоны областей»).
const (
	maxDrawingBytes   = 16 << 10
	minFreehandPoints = 2
	maxFreehandPoints = 2000
	maxDrawingText    = 500
	minTextSize       = 8
	maxTextSize       = 200
	minTemplateSize   = 5
	maxTemplateSize   = 1000
	maxRulerPoints    = 50
)

// The data of every kind, in the order the fields are written.
type (
	freehandData struct {
		Points [][]float64 `json:"points"`
	}
	freehandOut struct {
		Points [][2]float64 `json:"points"`
	}
	rectData struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		W float64 `json:"w"`
		H float64 `json:"h"`
	}
	circleData struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		R float64 `json:"r"`
	}
	textData struct {
		X    float64 `json:"x"`
		Y    float64 `json:"y"`
		Text string  `json:"text"`
		Size float64 `json:"size"`
	}
	templateData struct {
		Shape     string  `json:"shape"`
		X         float64 `json:"x"`
		Y         float64 `json:"y"`
		Size      float64 `json:"size"`
		Direction float64 `json:"direction"`
	}
)

// strictData decodes raw into dst: unknown fields and anything after the
// value are errors.
func strictData(raw json.RawMessage, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing data")
	}
	return nil
}

// validDrawingData checks data against the table of kind and returns it
// serialized again, so the stored and sent JSON is the normalized one: the
// order of the fields is that of the structure, a template direction is in
// [0, 360).
func validDrawingData(kind string, data json.RawMessage) (json.RawMessage, error) {
	if len(data) > maxDrawingBytes {
		return nil, invalid("data must be at most %d bytes", maxDrawingBytes)
	}
	var out any
	switch kind {
	case "freehand":
		var d freehandData
		if err := strictData(data, &d); err != nil {
			return nil, invalid("invalid freehand data")
		}
		if len(d.Points) < minFreehandPoints || len(d.Points) > maxFreehandPoints {
			return nil, invalid("freehand needs %d..%d points", minFreehandPoints, maxFreehandPoints)
		}
		o := freehandOut{Points: make([][2]float64, len(d.Points))}
		for i, p := range d.Points {
			if len(p) != 2 || !validCoord(p[0], p[1]) {
				return nil, invalid("a point must be [x, y] within ±%d", maxCoord)
			}
			o.Points[i] = [2]float64{p[0], p[1]}
		}
		out = o
	case "rect":
		var d rectData
		if err := strictData(data, &d); err != nil {
			return nil, invalid("invalid rect data")
		}
		if !validCoord(d.X, d.Y, d.W, d.H) || d.W <= 0 || d.H <= 0 {
			return nil, invalid("rect needs x, y within ±%d and w, h > 0", maxCoord)
		}
		out = d
	case "circle":
		var d circleData
		if err := strictData(data, &d); err != nil {
			return nil, invalid("invalid circle data")
		}
		if !validCoord(d.X, d.Y, d.R) || d.R <= 0 {
			return nil, invalid("circle needs x, y within ±%d and r > 0", maxCoord)
		}
		out = d
	case "text":
		var d textData
		if err := strictData(data, &d); err != nil {
			return nil, invalid("invalid text data")
		}
		if n := utf8.RuneCountInString(d.Text); n < 1 || n > maxDrawingText {
			return nil, invalid("text must be 1..%d characters", maxDrawingText)
		}
		if !validCoord(d.X, d.Y) || !finite(d.Size) || d.Size < minTextSize || d.Size > maxTextSize {
			return nil, invalid("text needs x, y within ±%d and size %d..%d", maxCoord, minTextSize, maxTextSize)
		}
		out = d
	case "template":
		var d templateData
		if err := strictData(data, &d); err != nil {
			return nil, invalid("invalid template data")
		}
		if !validEnum(d.Shape, "cone", "sphere", "line", "cube", "cylinder") {
			return nil, invalid("template shape must be cone, sphere, line, cube or cylinder")
		}
		if !validCoord(d.X, d.Y) || !finite(d.Size, d.Direction) || d.Size < minTemplateSize || d.Size > maxTemplateSize {
			return nil, invalid("template needs x, y within ±%d and size %d..%d", maxCoord, minTemplateSize, maxTemplateSize)
		}
		d.Direction = normalizeRotation(d.Direction)
		out = d
	default:
		return nil, invalid("kind must be freehand, rect, circle, text or template")
	}
	norm, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("encode drawing data: %w", err)
	}
	return norm, nil
}

const drawingSelect = `SELECT d.id, d.scene_id, d.kind, d.data, d.color, d.author_player_id FROM drawing d`

func scanDrawing(r rowScanner) (protocol.Drawing, error) {
	var (
		d    protocol.Drawing
		data string
	)
	if err := r.Scan(&d.ID, &d.SceneID, &d.Kind, &data, &d.Color, &d.AuthorPlayerID); err != nil {
		return protocol.Drawing{}, err
	}
	d.Data = json.RawMessage(data)
	return d, nil
}

// listDrawings reads every drawing of a scene, by id.
func listDrawings(ctx context.Context, q queryer, sceneID string) ([]protocol.Drawing, error) {
	rows, err := q.QueryContext(ctx, drawingSelect+` WHERE d.scene_id = ? ORDER BY d.id`, sceneID)
	if err != nil {
		return nil, fmt.Errorf("list drawings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []protocol.Drawing{}
	for rows.Next() {
		d, err := scanDrawing(rows)
		if err != nil {
			return nil, fmt.Errorf("scan drawing: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list drawings: %w", err)
	}
	return out, nil
}

// loadDrawing reads a drawing of the campaign and whether its scene is active;
// another campaign's drawing is ErrNotFound.
func loadDrawing(ctx context.Context, q queryer, campaignID, drawingID string) (protocol.Drawing, bool, error) {
	d, err := scanDrawing(q.QueryRowContext(ctx, drawingSelect+` JOIN scene s ON s.id = d.scene_id WHERE d.id = ? AND s.campaign_id = ?`, drawingID, campaignID))
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.Drawing{}, false, ErrNotFound
	}
	if err != nil {
		return protocol.Drawing{}, false, fmt.Errorf("load drawing: %w", err)
	}
	active, err := activeSceneID(ctx, q, campaignID)
	return d, active == d.SceneID, err
}

// DrawingResult is a drawing after a change and whether its scene was active
// at that moment; the renderer of the event captures both.
type DrawingResult struct {
	Drawing protocol.Drawing
	Active  bool
}

func errPlayerScene() error {
	return ws.Errorf(protocol.CodeForbidden, "only the active scene is open to players")
}

// CreateDrawing adds a drawing of v to a scene. A player may draw on the
// active scene only.
func (s *Service) CreateDrawing(ctx context.Context, campaignID string, v ws.Viewer, in protocol.DrawingCreate) (DrawingResult, error) {
	var res DrawingResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		active, err := sceneIsActive(ctx, tx, campaignID, in.SceneID)
		if err != nil {
			return err
		}
		if !v.IsGM && !active {
			return errPlayerScene()
		}
		data, err := validDrawingData(in.Kind, in.Data)
		if err != nil {
			return err
		}
		if !validColor(in.Color) {
			return invalid("color must be #rrggbb in lower case")
		}
		d := protocol.Drawing{ID: newID(), SceneID: in.SceneID, Kind: in.Kind, Data: data, Color: in.Color, AuthorPlayerID: v.PlayerID}
		nowMs := s.now().UnixMilli()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO drawing (id, scene_id, author_player_id, kind, data, color, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			d.ID, d.SceneID, d.AuthorPlayerID, d.Kind, string(d.Data), d.Color, nowMs, nowMs); err != nil {
			return fmt.Errorf("insert drawing: %w", err)
		}
		res = DrawingResult{Drawing: d, Active: active}
		return nil
	})
	return res, err
}

// mayChangeDrawing is the right to change or delete a drawing: the GM any; a
// player their own on the active scene.
func mayChangeDrawing(d protocol.Drawing, active bool, v ws.Viewer) error {
	if v.IsGM {
		return nil
	}
	if d.AuthorPlayerID != v.PlayerID || !active {
		return ws.Errorf(protocol.CodeForbidden, "you cannot change this drawing")
	}
	return nil
}

// UpdateDrawing changes the data and the color of a drawing; data is checked
// by the kind of the drawing.
func (s *Service) UpdateDrawing(ctx context.Context, campaignID string, v ws.Viewer, in protocol.DrawingUpdate) (DrawingResult, error) {
	var res DrawingResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		d, active, err := loadDrawing(ctx, tx, campaignID, in.DrawingID)
		if err != nil {
			return err
		}
		if err := mayChangeDrawing(d, active, v); err != nil {
			return err
		}
		if len(in.Data) > 0 {
			if d.Data, err = validDrawingData(d.Kind, in.Data); err != nil {
				return err
			}
		}
		if in.Color != nil {
			if !validColor(*in.Color) {
				return invalid("color must be #rrggbb in lower case")
			}
			d.Color = *in.Color
		}
		if _, err := tx.ExecContext(ctx, `UPDATE drawing SET data = ?, color = ?, updated_at = ? WHERE id = ?`,
			string(d.Data), d.Color, s.now().UnixMilli(), d.ID); err != nil {
			return fmt.Errorf("update drawing: %w", err)
		}
		res = DrawingResult{Drawing: d, Active: active}
		return nil
	})
	return res, err
}

// DeleteDrawing removes a drawing.
func (s *Service) DeleteDrawing(ctx context.Context, campaignID string, v ws.Viewer, drawingID string) (DrawingResult, error) {
	var res DrawingResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		d, active, err := loadDrawing(ctx, tx, campaignID, drawingID)
		if err != nil {
			return err
		}
		if err := mayChangeDrawing(d, active, v); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM drawing WHERE id = ?`, d.ID); err != nil {
			return fmt.Errorf("delete drawing: %w", err)
		}
		res = DrawingResult{Drawing: d, Active: active}
		return nil
	})
	return res, err
}

// sceneIsActive reads whether a scene of the campaign is the active one; a
// scene of another campaign is ErrNotFound.
func sceneIsActive(ctx context.Context, q queryer, campaignID, sceneID string) (bool, error) {
	if _, err := loadScene(ctx, q, campaignID, sceneID); err != nil {
		return false, err
	}
	active, err := activeSceneID(ctx, q, campaignID)
	return active == sceneID, err
}

// SceneActive is sceneIsActive for ping and ruler: it only reads.
func (s *Service) SceneActive(ctx context.Context, campaignID, sceneID string) (bool, error) {
	return sceneIsActive(ctx, s.db, campaignID, sceneID)
}

// validRulerPoints reports whether the points of a ruler are 1..maxRulerPoints
// finite coordinates within ±maxCoord.
func validRulerPoints(pts [][2]float64) bool {
	if len(pts) < 1 || len(pts) > maxRulerPoints {
		return false
	}
	for _, p := range pts {
		if !validCoord(p[0], p[1]) {
			return false
		}
	}
	return true
}
