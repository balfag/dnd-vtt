package vtt

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"

	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// Limits of the fog (protocol.md, «Туман»).
const (
	maxFogOps        = 500
	maxPolygonPoints = 200
	maxCoord         = 100000
)

// edgeEpsilon is how close to a polygon edge a point counts as on it.
const edgeEpsilon = 1e-9

func validCoord(ns ...float64) bool {
	for _, n := range ns {
		if !finite(n) || math.Abs(n) > maxCoord {
			return false
		}
	}
	return true
}

// validShape checks finite coordinates within ±maxCoord; rect w, h > 0;
// circle r > 0; polygon 3..maxPolygonPoints points.
func validShape(s protocol.Shape) bool {
	switch s.Type {
	case protocol.ShapeRect:
		return validCoord(s.X, s.Y, s.W, s.H) && s.W > 0 && s.H > 0
	case protocol.ShapeCircle:
		return validCoord(s.X, s.Y, s.R) && s.R > 0
	case protocol.ShapePolygon:
		if len(s.Points) < 3 || len(s.Points) > maxPolygonPoints {
			return false
		}
		for _, p := range s.Points {
			if !validCoord(p[0], p[1]) {
				return false
			}
		}
		return true
	}
	return false
}

// contains reports whether point (x, y) is inside s; the boundary counts as inside.
func contains(s protocol.Shape, x, y float64) bool {
	switch s.Type {
	case protocol.ShapeRect:
		return s.X <= x && x <= s.X+s.W && s.Y <= y && y <= s.Y+s.H
	case protocol.ShapeCircle:
		return math.Hypot(x-s.X, y-s.Y) <= s.R
	case protocol.ShapePolygon:
		return inPolygon(s.Points, x, y)
	}
	return false
}

// inPolygon is the even-odd rule, after a check for a point on an edge: the
// rule alone is unstable there.
func inPolygon(pts [][2]float64, x, y float64) bool {
	n := len(pts)
	for i := range n {
		if distToSegment(x, y, pts[i], pts[(i+1)%n]) <= edgeEpsilon {
			return true
		}
	}
	inside := false
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		a, b := pts[i], pts[j]
		if (a[1] > y) != (b[1] > y) && x < (b[0]-a[0])*(y-a[1])/(b[1]-a[1])+a[0] {
			inside = !inside
		}
	}
	return inside
}

// distToSegment is the distance from (x, y) to the segment ab.
func distToSegment(x, y float64, a, b [2]float64) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(x-a[0], y-a[1])
	}
	t := max(0, min(1, ((x-a[0])*dx+(y-a[1])*dy)/l2))
	return math.Hypot(x-(a[0]+t*dx), y-(a[1]+t*dy))
}

// revealed reports whether point (x, y) is revealed: fog disabled -> true;
// otherwise the last op in order whose shape contains the point decides
// (reveal -> true, hide -> false); a point in no shape is hidden.
func revealed(f protocol.Fog, x, y float64) bool {
	if !f.Enabled {
		return true
	}
	for i := len(f.Ops) - 1; i >= 0; i-- {
		if contains(f.Ops[i].Shape, x, y) {
			return f.Ops[i].Op == "reveal"
		}
	}
	return false
}

// FogResult is a scene's fog after a change, with every token of the scene and
// whether the scene was active at that moment; the renderer of fog.changed
// captures all of it.
type FogResult struct {
	SceneID string
	Active  bool
	Fog     protocol.Fog
	Tokens  []protocol.Token
}

// SetFog switches the fog of a scene on or off.
func (s *Service) SetFog(ctx context.Context, campaignID string, in protocol.FogSet) (FogResult, error) {
	return s.changeFog(ctx, campaignID, in.SceneID, func(f *protocol.Fog) error {
		f.Enabled = in.Enabled
		return nil
	})
}

// AddFogOp appends a reveal or hide operation to the fog of a scene.
func (s *Service) AddFogOp(ctx context.Context, campaignID, op string, in protocol.FogShape) (FogResult, error) {
	if !validShape(in.Shape) {
		return FogResult{}, invalid("invalid shape")
	}
	return s.changeFog(ctx, campaignID, in.SceneID, func(f *protocol.Fog) error {
		if len(f.Ops) >= maxFogOps {
			return fmt.Errorf("%w: a scene has at most %d fog operations", ErrConflict, maxFogOps)
		}
		f.Ops = append(f.Ops, protocol.FogOp{Op: op, Shape: in.Shape})
		return nil
	})
}

// ResetFog removes every operation of the fog of a scene; enabled stays.
func (s *Service) ResetFog(ctx context.Context, campaignID, sceneID string) (FogResult, error) {
	return s.changeFog(ctx, campaignID, sceneID, func(f *protocol.Fog) error {
		f.Ops = []protocol.FogOp{}
		return nil
	})
}

// changeFog reads the fog of the scene, applies change and writes it back in
// one transaction, then reads the tokens of the scene.
func (s *Service) changeFog(ctx context.Context, campaignID, sceneID string, change func(*protocol.Fog) error) (FogResult, error) {
	var res FogResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		row, err := loadScene(ctx, tx, campaignID, sceneID)
		if err != nil {
			return err
		}
		fog := row.fog
		fog.Ops = append([]protocol.FogOp{}, fog.Ops...)
		if err := change(&fog); err != nil {
			return err
		}
		raw, err := json.Marshal(fog)
		if err != nil {
			return fmt.Errorf("encode fog: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE scene SET fog = ?, updated_at = ? WHERE id = ?`, string(raw), s.now().UnixMilli(), sceneID); err != nil {
			return fmt.Errorf("update fog: %w", err)
		}
		active, err := activeSceneID(ctx, tx, campaignID)
		if err != nil {
			return err
		}
		tokens, err := listTokens(ctx, tx, sceneID)
		if err != nil {
			return err
		}
		res = FogResult{SceneID: sceneID, Active: active == sceneID, Fog: fog, Tokens: tokens}
		return nil
	})
	return res, err
}
