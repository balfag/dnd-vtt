package vtt

import (
	"context"
	"encoding/json"

	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// combatChanged shows the GM the whole tracker. A player gets it for the
// active scene only, without hidden participants; an ended fight is null.
func combatChanged(res CombatResult) ws.Renderer {
	return func(v ws.Viewer) (string, any, bool) {
		if !v.IsGM && !res.Active {
			return "", nil, false
		}
		d := protocol.CombatChanged{SceneID: res.SceneID}
		if res.Combat != nil {
			c := res.Combat.For(v)
			d.Combat = &c
		}
		return protocol.TypeCombatChanged, d, true
	}
}

// emitCombat answers a combat command: the tracker goes to everyone, the ack is empty.
func emitCombat(em ws.Emitter, res CombatResult, err error) (any, error) {
	if err != nil {
		return nil, commandError(err)
	}
	em.Emit(combatChanged(res))
	return nil, nil
}

func (h *handlers) combatStart(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	if err := gmOnly(cmd); err != nil {
		return nil, err
	}
	var in protocol.SceneRef
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	res, err := h.s.StartCombat(ctx, cmd.CampaignID, in.SceneID)
	return emitCombat(em, res, err)
}

func (h *handlers) combatEnd(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	if err := gmOnly(cmd); err != nil {
		return nil, err
	}
	var in protocol.SceneRef
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	res, err := h.s.EndCombat(ctx, cmd.CampaignID, in.SceneID)
	return emitCombat(em, res, err)
}

func (h *handlers) combatAdd(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	if err := gmOnly(cmd); err != nil {
		return nil, err
	}
	var in protocol.CombatAdd
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	res, err := h.s.AddCombatants(ctx, cmd.CampaignID, in)
	return emitCombat(em, res, err)
}

func (h *handlers) combatRemove(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	if err := gmOnly(cmd); err != nil {
		return nil, err
	}
	var in protocol.CombatantRef
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	res, err := h.s.RemoveCombatant(ctx, cmd.CampaignID, in.CombatantID)
	return emitCombat(em, res, err)
}

func (h *handlers) combatSetInitiative(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	var in protocol.CombatSetInitiative
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	// value is required: null clears, and an absent field is an error.
	var probe struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(cmd.D, &probe); err != nil || len(probe.Value) == 0 {
		return nil, ws.Errorf(protocol.CodeBadRequest, "value is required: an integer or null")
	}
	res, err := h.s.SetInitiative(ctx, cmd.CampaignID, cmd.Viewer, in.CombatantID, in.Value)
	return emitCombat(em, res, err)
}

func (h *handlers) combatRollInitiative(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	var in protocol.CombatRollInitiative
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	res, err := h.s.RollInitiative(ctx, cmd.CampaignID, cmd.Viewer, in.CombatantIDs, h.roller)
	return emitCombat(em, res, err)
}

func (h *handlers) combatSetHidden(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	if err := gmOnly(cmd); err != nil {
		return nil, err
	}
	var in protocol.CombatSetHidden
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	res, err := h.s.SetCombatantHidden(ctx, cmd.CampaignID, in.CombatantID, in.Hidden)
	return emitCombat(em, res, err)
}

func (h *handlers) combatNext(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	var in protocol.SceneRef
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	res, err := h.s.NextTurn(ctx, cmd.CampaignID, cmd.Viewer, in.SceneID)
	return emitCombat(em, res, err)
}

func (h *handlers) combatPrev(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	if err := gmOnly(cmd); err != nil {
		return nil, err
	}
	var in protocol.SceneRef
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	res, err := h.s.PrevTurn(ctx, cmd.CampaignID, cmd.Viewer, in.SceneID)
	return emitCombat(em, res, err)
}
