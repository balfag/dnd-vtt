package vtt

import (
	"slices"

	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// sceneState is what visibility needs to know about a token's scene at emit time.
type sceneState struct {
	Active bool // the scene is the campaign's active scene
	// #51 adds the fog here.
}

// TokenVisible reports whether v sees t (protocol.md, «Видимость фишки для игрока»):
// the GM sees everything; a player sees tokens of the active scene only, never
// layer gm; an owner sees own tokens even when hidden; others see tokens that
// are not hidden. #51 adds the fog condition for non-owners.
func TokenVisible(t protocol.Token, v ws.Viewer, s sceneState) bool {
	if v.IsGM {
		return true
	}
	if !s.Active || t.Layer == "gm" {
		return false
	}
	if owns(t, v) {
		return true
	}
	return !t.Hidden
}

func owns(t protocol.Token, v ws.Viewer) bool {
	return v.PlayerID != "" && slices.Contains(t.Owners, v.PlayerID)
}

// RenderToken returns t as v sees it; the caller has checked TokenVisible.
// For a player: name is "" when nameVisibility is gm; bars keep visibility
// all, plus owners when v owns t; gm bars are dropped. The GM gets t unchanged.
// It never modifies t (slices are copied).
func RenderToken(t protocol.Token, v ws.Viewer) protocol.Token {
	out := t
	out.Owners = slices.Clone(t.Owners)
	out.Conditions = slices.Clone(t.Conditions)
	if out.Owners == nil {
		out.Owners = []string{}
	}
	if out.Conditions == nil {
		out.Conditions = []string{}
	}
	if t.Image != nil {
		img := *t.Image
		out.Image = &img
	}
	bars := make([]protocol.TokenBar, 0, len(t.Bars))
	for _, b := range t.Bars {
		if v.IsGM || b.Visibility == "all" || (b.Visibility == "owners" && owns(t, v)) {
			bars = append(bars, b)
		}
	}
	out.Bars = bars
	if !v.IsGM && t.NameVisibility == "gm" {
		out.Name = ""
	}
	return out
}

// visibleTokens returns the tokens v sees, rendered for v, in the order given.
func visibleTokens(all []protocol.Token, v ws.Viewer, s sceneState) []protocol.Token {
	out := make([]protocol.Token, 0, len(all))
	for _, t := range all {
		if TokenVisible(t, v, s) {
			out = append(out, RenderToken(t, v))
		}
	}
	return out
}

// sceneFor returns scene (whose Tokens hold every token of the scene) as v
// sees it. It copies the scene, so the original stays whole for other viewers.
func sceneFor(scene protocol.SceneView, v ws.Viewer, s sceneState) protocol.SceneView {
	scene.Tokens = visibleTokens(scene.Tokens, v, s)
	return scene
}
