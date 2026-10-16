package vtt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// Register adds the scene.* handlers to hub.
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
	d := protocol.SceneChanged{Scene: scene}
	return func(v ws.Viewer) (string, any, bool) {
		return protocol.TypeSceneChanged, d, v.IsGM || wasActive
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
		em.EmitAll(protocol.TypeSceneActivated, protocol.SceneActivated{Scene: *res.Activated})
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
	// Everyone gets the same for now; with tokens in #50 it becomes a Renderer.
	em.EmitAll(protocol.TypeSceneActivated, protocol.SceneActivated{Scene: scene})
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
