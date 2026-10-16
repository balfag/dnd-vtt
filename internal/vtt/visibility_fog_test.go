package vtt

import (
	"testing"

	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

func TestTokenVisibleInTheFog(t *testing.T) {
	owner := ws.Viewer{PlayerID: "owner"}
	other := ws.Viewer{PlayerID: "other"}
	gm := ws.Viewer{IsGM: true}
	revealedArea := protocol.Shape{Type: protocol.ShapeRect, X: 0, Y: 0, W: 100, H: 100}
	fogOn := protocol.Fog{Enabled: true, Ops: []protocol.FogOp{{Op: "reveal", Shape: revealedArea}}}
	fogOff := protocol.Fog{Enabled: false, Ops: fogOn.Ops}

	for _, tt := range []struct {
		name string
		fog  protocol.Fog
		x, y float64
		v    ws.Viewer
		want bool
	}{
		{"other, revealed area, fog on", fogOn, 50, 50, other, true},
		{"other, border of the revealed area", fogOn, 100, 100, other, true},
		{"other, hidden area, fog on", fogOn, 500, 500, other, false},
		{"other, hidden area, fog off", fogOff, 500, 500, other, true},
		{"other, revealed area, fog off", fogOff, 50, 50, other, true},
		{"owner, hidden area, fog on", fogOn, 500, 500, owner, true},
		{"owner, revealed area, fog on", fogOn, 50, 50, owner, true},
		{"gm, hidden area, fog on", fogOn, 500, 500, gm, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tok := protocol.Token{ID: "t", Layer: "tokens", X: tt.x, Y: tt.y, Owners: []string{"owner"}}
			if got := TokenVisible(tok, tt.v, sceneState{Active: true, Fog: tt.fog}); got != tt.want {
				t.Errorf("TokenVisible = %t, want %t", got, tt.want)
			}
		})
	}

	t.Run("a hidden token stays hidden in a revealed area", func(t *testing.T) {
		tok := protocol.Token{ID: "t", Layer: "tokens", X: 50, Y: 50, Hidden: true}
		if TokenVisible(tok, other, sceneState{Active: true, Fog: fogOn}) {
			t.Error("a hidden token is visible")
		}
	})
}
