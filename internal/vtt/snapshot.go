package vtt

import (
	"context"

	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// FillSnapshot implements ws.SnapshotSource: activeSceneId, scenes and scene.
// The GM gets every scene of the campaign; a player gets the active scene only.
func (s *Service) FillSnapshot(ctx context.Context, v ws.Viewer, campaignID string, t *protocol.SnapshotTable) error {
	active, err := activeSceneID(ctx, s.db, campaignID)
	if err != nil {
		return err
	}
	if active != "" {
		row, err := loadScene(ctx, s.db, campaignID, active)
		if err != nil {
			return err
		}
		view := row.view()
		all, err := listTokens(ctx, s.db, active)
		if err != nil {
			return err
		}
		view.Tokens = all
		view = sceneFor(view, v, sceneState{Active: true, Fog: view.Fog})
		t.ActiveSceneID = &active
		t.Scene = &view
	}
	if t.Combat, err = activeCombat(ctx, s.db, campaignID, v); err != nil {
		return err
	}
	if v.IsGM {
		t.Scenes, err = listScenes(ctx, s.db, campaignID)
		if err != nil {
			return err
		}
	}
	return nil
}
