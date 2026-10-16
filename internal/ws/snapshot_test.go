package ws

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

type sourceFunc func(ctx context.Context, v Viewer, campaignID string, t *protocol.SnapshotTable) error

func (f sourceFunc) FillSnapshot(ctx context.Context, v Viewer, campaignID string, t *protocol.SnapshotTable) error {
	return f(ctx, v, campaignID, t)
}

func TestSnapshotTableIsEmptyWithoutSources(t *testing.T) {
	f := newFixture(t, testOptions())
	c := f.dialAs(f.player)
	write(t, c, `{"t":"sync.hello","d":{}}`)
	env := read(t, c)
	for _, want := range []string{`"activeSceneId":null`, `"scenes":[]`, `"scene":null`} {
		if !strings.Contains(string(env.D), want) {
			t.Errorf("snapshot %s lacks %s", env.D, want)
		}
	}
}

func TestSnapshotSourcesRunInOrderForTheViewer(t *testing.T) {
	f := newFixture(t, testOptions())
	var order []string
	var gotViewer Viewer
	var gotCampaign string
	f.hub.AddSnapshotSource(sourceFunc(func(_ context.Context, v Viewer, campaignID string, tbl *protocol.SnapshotTable) error {
		order = append(order, "first")
		gotViewer, gotCampaign = v, campaignID
		id := "scene-1"
		tbl.ActiveSceneID = &id
		return nil
	}))
	f.hub.AddSnapshotSource(sourceFunc(func(_ context.Context, _ Viewer, _ string, tbl *protocol.SnapshotTable) error {
		order = append(order, "second")
		if tbl.ActiveSceneID == nil {
			t.Error("the second source does not see the work of the first")
		}
		return nil
	}))

	snap := f.hello(f.dialAs(f.player))
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Errorf("order = %v", order)
	}
	if gotViewer.IsGM || gotViewer.PlayerID != f.playerID || gotCampaign != f.camp.ID {
		t.Errorf("source got %+v in campaign %q", gotViewer, gotCampaign)
	}
	if snap.Game == nil || snap.Game.ActiveSceneID == nil || *snap.Game.ActiveSceneID != "scene-1" {
		t.Errorf("snapshot = %+v", snap.Game)
	}
	if snap.Game.Scenes == nil {
		t.Error("scenes is nil: it must stay an empty array")
	}
}

func TestSnapshotSourceErrorIsInternal(t *testing.T) {
	f := newFixture(t, testOptions())
	f.hub.AddSnapshotSource(sourceFunc(func(context.Context, Viewer, string, *protocol.SnapshotTable) error {
		return errors.New("db is down")
	}))
	c := f.dialAs(f.player)
	write(t, c, `{"t":"sync.hello","id":"c-1","d":{}}`)
	wantError(t, read(t, c), "c-1", protocol.CodeInternal)
}
