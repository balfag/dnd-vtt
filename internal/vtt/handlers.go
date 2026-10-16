package vtt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// Register adds the scene.* and token.* handlers to hub.
func Register(hub *ws.Hub, s *Service) {
	h := &handlers{s: s}
	for t, fn := range map[string]ws.Handler{
		protocol.TypeSceneCreate:   h.create,
		protocol.TypeSceneUpdate:   h.update,
		protocol.TypeSceneDelete:   h.delete,
		protocol.TypeSceneReorder:  h.reorder,
		protocol.TypeSceneActivate: h.activate,
		protocol.TypeSceneView:     h.view,
	} {
		hub.Handle(t, fn, ws.HandlerOptions{})
	}
	for t, fn := range map[string]ws.Handler{
		protocol.TypeTokenCreate: h.tokenCreate,
		protocol.TypeTokenUpdate: h.tokenUpdate,
		protocol.TypeTokenMove:   h.tokenMove,
		protocol.TypeTokenDelete: h.tokenDelete,
	} {
		hub.Handle(t, fn, ws.HandlerOptions{})
	}
	// Dragging is frequent and has no reply: over the limit it is just dropped.
	hub.Handle(protocol.TypeTokenDrag, h.tokenDrag, ws.HandlerOptions{RatePerSec: 30, Silent: true})
}

type handlers struct{ s *Service }

// decode reads the data of a command strictly: unknown fields are an error.
func decode(cmd ws.Command, dst any) error {
	if len(cmd.D) == 0 {
		return ws.Errorf(protocol.CodeBadRequest, "%s needs data", cmd.T)
	}
	dec := json.NewDecoder(bytes.NewReader(cmd.D))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return ws.Errorf(protocol.CodeBadRequest, "invalid data of %s", cmd.T)
	}
	return nil
}

// gmOnly checks the right to run a scene command; players get forbidden.
func gmOnly(cmd ws.Command) error {
	if !cmd.Viewer.IsGM {
		return ws.Errorf(protocol.CodeForbidden, "only the gm can do this")
	}
	return nil
}

// commandError turns a service error into the reply of the command.
func commandError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return ws.Errorf(protocol.CodeNotFound, "not found")
	case errors.Is(err, ErrInvalid):
		return ws.Errorf(protocol.CodeBadRequest, "%s", err.Error())
	case errors.Is(err, ErrConflict):
		return ws.Errorf(protocol.CodeConflict, "%s", err.Error())
	}
	return err // an unexpected failure: the dispatcher answers internal
}

// toGM is the renderer of an event only the GM sees; players get sync.noop.
func toGM(t string, d any) ws.Renderer {
	return func(v ws.Viewer) (string, any, bool) { return t, d, v.IsGM }
}

func scenesChanged(scenes []protocol.SceneListItem) ws.Renderer {
	return toGM(protocol.TypeScenesChanged, protocol.ScenesChanged{Scenes: scenes})
}

// sceneChanged shows the GM every change and a player only that of the scene
// that was active when it changed.
func sceneChanged(scene protocol.SceneView, wasActive bool) ws.Renderer {
	d := protocol.SceneChanged{Scene: scene.Info()}
	return func(v ws.Viewer) (string, any, bool) {
		return protocol.TypeSceneChanged, d, v.IsGM || wasActive
	}
}

// sceneActivated gives everyone the new active scene with the tokens they may
// see; scene holds all of them and is not changed.
func sceneActivated(scene protocol.SceneView) ws.Renderer {
	return func(v ws.Viewer) (string, any, bool) {
		return protocol.TypeSceneActivated, protocol.SceneActivated{Scene: sceneFor(scene, v, sceneState{Active: true})}, true
	}
}

func (h *handlers) create(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	if err := gmOnly(cmd); err != nil {
		return nil, err
	}
	var in protocol.SceneCreate
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	res, err := h.s.CreateScene(ctx, cmd.CampaignID, in)
	if err != nil {
		return nil, commandError(err)
	}
	em.Emit(scenesChanged(res.Scenes))
	if res.Activated != nil {
		em.Emit(sceneActivated(*res.Activated))
	}
	return protocol.SceneCreateAck{SceneID: res.SceneID}, nil
}

func (h *handlers) update(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	if err := gmOnly(cmd); err != nil {
		return nil, err
	}
	var in protocol.SceneUpdate
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	res, err := h.s.UpdateScene(ctx, cmd.CampaignID, in)
	if err != nil {
		return nil, commandError(err)
	}
	em.Emit(sceneChanged(res.Scene, res.WasActive))
	return nil, nil
}

func (h *handlers) delete(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	if err := gmOnly(cmd); err != nil {
		return nil, err
	}
	var in protocol.SceneRef
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	scenes, err := h.s.DeleteScene(ctx, cmd.CampaignID, in.SceneID)
	if err != nil {
		return nil, commandError(err)
	}
	em.Emit(scenesChanged(scenes))
	return nil, nil
}

func (h *handlers) reorder(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	if err := gmOnly(cmd); err != nil {
		return nil, err
	}
	var in protocol.SceneReorder
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	scenes, err := h.s.ReorderScenes(ctx, cmd.CampaignID, in.SceneIDs)
	if err != nil {
		return nil, commandError(err)
	}
	em.Emit(scenesChanged(scenes))
	return nil, nil
}

func (h *handlers) activate(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	if err := gmOnly(cmd); err != nil {
		return nil, err
	}
	var in protocol.SceneRef
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	scene, err := h.s.ActivateScene(ctx, cmd.CampaignID, in.SceneID)
	if err != nil {
		return nil, commandError(err)
	}
	em.Emit(sceneActivated(scene))
	return nil, nil
}

func (h *handlers) view(ctx context.Context, cmd ws.Command, _ ws.Emitter) (any, error) {
	if err := gmOnly(cmd); err != nil {
		return nil, err
	}
	var in protocol.SceneRef
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	scene, err := h.s.ViewScene(ctx, cmd.CampaignID, in.SceneID)
	if err != nil {
		return nil, commandError(err)
	}
	return protocol.SceneViewAck{Scene: scene}, nil
}

// tokenChanged shows the GM every change. A player gets it only for the active
// scene: the token as the player may see it, or token.removed when the change
// hid it. The token and the scene state are those of the moment of the change.
func tokenChanged(t protocol.Token, st sceneState) ws.Renderer {
	return func(v ws.Viewer) (string, any, bool) {
		switch {
		case v.IsGM:
			return protocol.TypeTokenChanged, protocol.TokenChanged{Token: t}, true
		case !st.Active:
			return "", nil, false
		case TokenVisible(t, v, st):
			return protocol.TypeTokenChanged, protocol.TokenChanged{Token: RenderToken(t, v)}, true
		}
		return protocol.TypeTokenRemoved, protocol.TokenRef{TokenID: t.ID}, true
	}
}

// tokenRemoved tells the GM always and a player when the scene is active, even
// if the player never saw the token: the client ignores an unknown id.
func tokenRemoved(tokenID string, st sceneState) ws.Renderer {
	return func(v ws.Viewer) (string, any, bool) {
		return protocol.TypeTokenRemoved, protocol.TokenRef{TokenID: tokenID}, v.IsGM || st.Active
	}
}

// mayMove is the right to move a token: the GM any; a player a token they own,
// which is not locked, on the active scene. A token the player cannot see is
// reported as not found.
func mayMove(v ws.Viewer) func(protocol.Token, sceneState) error {
	return func(t protocol.Token, st sceneState) error {
		switch {
		case v.IsGM:
			return nil
		case !TokenVisible(t, v, st):
			return ErrNotFound
		case !owns(t, v) || t.Locked || !st.Active:
			return ws.Errorf(protocol.CodeForbidden, "you cannot move this token")
		}
		return nil
	}
}

func (h *handlers) tokenCreate(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	if err := gmOnly(cmd); err != nil {
		return nil, err
	}
	var in protocol.TokenCreate
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	res, err := h.s.CreateToken(ctx, cmd.CampaignID, in)
	if err != nil {
		return nil, commandError(err)
	}
	em.Emit(tokenChanged(res.Token, res.State))
	return protocol.TokenCreateAck{TokenID: res.Token.ID}, nil
}

func (h *handlers) tokenUpdate(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	if err := gmOnly(cmd); err != nil {
		return nil, err
	}
	var in protocol.TokenUpdate
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	res, err := h.s.UpdateToken(ctx, cmd.CampaignID, in)
	if err != nil {
		return nil, commandError(err)
	}
	em.Emit(tokenChanged(res.Token, res.State))
	return nil, nil
}

func (h *handlers) tokenMove(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	var in protocol.TokenMove
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	res, err := h.s.MoveToken(ctx, cmd.CampaignID, in, mayMove(cmd.Viewer))
	if err != nil {
		return nil, commandError(err)
	}
	em.Emit(tokenChanged(res.Token, res.State))
	return nil, nil
}

// tokenDrag shows a drag in progress to those who see the token. Nothing is
// stored and no reply is sent, whatever is wrong with the command.
func (h *handlers) tokenDrag(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	var in protocol.TokenMove
	if err := decode(cmd, &in); err != nil {
		return nil, nil
	}
	res, ok, err := h.s.CheckDrag(ctx, cmd.CampaignID, in, mayMove(cmd.Viewer))
	if err != nil || !ok {
		return nil, nil
	}
	em.Transient(protocol.TypeTokenDragging, in, func(v ws.Viewer) bool {
		return TokenVisible(res.Token, v, res.State)
	})
	return nil, nil
}

func (h *handlers) tokenDelete(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	if err := gmOnly(cmd); err != nil {
		return nil, err
	}
	var in protocol.TokenRef
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	res, err := h.s.DeleteToken(ctx, cmd.CampaignID, in.TokenID)
	if err != nil {
		return nil, commandError(err)
	}
	em.Emit(tokenRemoved(res.Token.ID, res.State))
	return nil, nil
}
