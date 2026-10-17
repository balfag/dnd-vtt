package chat

import (
	"context"

	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// FillSnapshot implements ws.SnapshotSource: the last 50 messages visible to v.
func (s *Service) FillSnapshot(ctx context.Context, v ws.Viewer, campaignID string, t *protocol.SnapshotTable) error {
	msgs, err := s.visible(ctx, v, campaignID, "", snapshotLimit)
	if err != nil {
		return err
	}
	t.Chat = msgs
	return nil
}
