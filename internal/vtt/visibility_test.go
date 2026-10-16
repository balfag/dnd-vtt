package vtt

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

func TestTokenVisible(t *testing.T) {
	gm := ws.Viewer{IsGM: true, PlayerID: "gm-player"}
	owner := ws.Viewer{PlayerID: "owner"}
	other := ws.Viewer{PlayerID: "other"}
	viewers := []struct {
		name string
		v    ws.Viewer
	}{{"gm", gm}, {"owner", owner}, {"other", other}}

	for _, layer := range []string{"map", "tokens", "gm"} {
		for _, hidden := range []bool{false, true} {
			for _, active := range []bool{true, false} {
				tok := protocol.Token{ID: "t", Layer: layer, Hidden: hidden, Owners: []string{"owner"}}
				st := sceneState{Active: active}
				want := map[string]bool{
					"gm":    true,
					"owner": active && layer != "gm",
					"other": active && layer != "gm" && !hidden,
				}
				for _, vw := range viewers {
					name := fmt.Sprintf("%s/layer=%s/hidden=%t/active=%t", vw.name, layer, hidden, active)
					t.Run(name, func(t *testing.T) {
						if got := TokenVisible(tok, vw.v, st); got != want[vw.name] {
							t.Errorf("TokenVisible = %t, want %t", got, want[vw.name])
						}
					})
				}
			}
		}
	}
}

func TestTokenVisibleWithoutPlayerID(t *testing.T) {
	// A GM outside the campaign roster has no PlayerID; an empty id is never an owner.
	tok := protocol.Token{Layer: "tokens", Hidden: true, Owners: []string{""}}
	if TokenVisible(tok, ws.Viewer{}, sceneState{Active: true}) {
		t.Error("a viewer without a player id sees a hidden token")
	}
}

func sampleToken() protocol.Token {
	return protocol.Token{
		ID: "t", Layer: "tokens", Name: "Goblin", NameVisibility: "hover",
		Image:  &protocol.AssetRef{AssetID: "a", URL: "/assets/a"},
		Owners: []string{"owner"},
		Bars: []protocol.TokenBar{
			{Key: "hp", Value: 5, Max: 7, Visibility: "owners"},
			{Key: "ac", Value: 15, Max: 15, Visibility: "all"},
			{Key: "secret", Value: 1, Max: 3, Visibility: "gm"},
		},
		Conditions: []string{"prone"},
	}
}

func barKeys(t protocol.Token) []string {
	keys := []string{}
	for _, b := range t.Bars {
		keys = append(keys, b.Key)
	}
	return keys
}

func TestRenderTokenName(t *testing.T) {
	player := ws.Viewer{PlayerID: "other"}
	owner := ws.Viewer{PlayerID: "owner"}
	for _, tt := range []struct {
		visibility string
		v          ws.Viewer
		want       string
	}{
		{"all", player, "Goblin"},
		{"hover", player, "Goblin"},
		{"gm", player, ""},
		{"gm", owner, ""},
		{"gm", ws.Viewer{IsGM: true}, "Goblin"},
	} {
		tok := sampleToken()
		tok.NameVisibility = tt.visibility
		if got := RenderToken(tok, tt.v).Name; got != tt.want {
			t.Errorf("nameVisibility %s, viewer %+v: name = %q, want %q", tt.visibility, tt.v, got, tt.want)
		}
	}
}

func TestRenderTokenBars(t *testing.T) {
	tok := sampleToken()
	for _, tt := range []struct {
		name string
		v    ws.Viewer
		want []string
	}{
		{"gm", ws.Viewer{IsGM: true}, []string{"hp", "ac", "secret"}},
		{"owner", ws.Viewer{PlayerID: "owner"}, []string{"hp", "ac"}},
		{"other", ws.Viewer{PlayerID: "other"}, []string{"ac"}},
	} {
		if got := barKeys(RenderToken(tok, tt.v)); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s sees bars %v, want %v", tt.name, got, tt.want)
		}
	}
	tok.Bars = []protocol.TokenBar{{Key: "secret", Value: 1, Max: 3, Visibility: "gm"}}
	if got := RenderToken(tok, ws.Viewer{PlayerID: "other"}).Bars; got == nil || len(got) != 0 {
		t.Errorf("bars = %#v, want an empty non-nil list", got)
	}
}

func TestRenderTokenDoesNotChangeTheOriginal(t *testing.T) {
	orig := sampleToken()
	want := sampleToken()
	for _, v := range []ws.Viewer{{IsGM: true}, {PlayerID: "owner"}, {PlayerID: "other"}} {
		got := RenderToken(orig, v)
		got.Owners[0] = "changed"
		got.Conditions[0] = "changed"
		got.Image.AssetID = "changed"
		if len(got.Bars) > 0 {
			got.Bars[0].Value = -1
		}
		got.Name = "changed"
		if !reflect.DeepEqual(orig, want) {
			t.Fatalf("RenderToken for %+v changed the original: %+v", v, orig)
		}
	}
}

func TestSceneForKeepsTheOriginalWhole(t *testing.T) {
	scene := protocol.SceneView{Tokens: []protocol.Token{
		{ID: "a", Layer: "tokens", Name: "A", NameVisibility: "gm", Owners: []string{}, Bars: []protocol.TokenBar{}, Conditions: []string{}},
		{ID: "b", Layer: "tokens", Hidden: true, Owners: []string{}, Bars: []protocol.TokenBar{}, Conditions: []string{}},
	}}
	got := sceneFor(scene, ws.Viewer{PlayerID: "p"}, sceneState{Active: true})
	if len(got.Tokens) != 1 || got.Tokens[0].ID != "a" || got.Tokens[0].Name != "" {
		t.Errorf("player tokens = %+v", got.Tokens)
	}
	if len(scene.Tokens) != 2 || scene.Tokens[0].Name != "A" {
		t.Errorf("the original scene changed: %+v", scene.Tokens)
	}
}
