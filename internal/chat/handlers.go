package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/balfag/dnd-vtt/internal/dice"
	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// Register adds chat.send, roll.request, chat.history and handout.show to hub; roller rolls the dice.
func Register(hub *ws.Hub, s *Service, roller dice.Roller) {
	h := &handlers{s: s, roller: roller}
	hub.Handle(protocol.TypeChatSend, h.send, ws.HandlerOptions{})
	hub.Handle(protocol.TypeRollRequest, h.roll, ws.HandlerOptions{RatePerSec: 5})
	hub.Handle(protocol.TypeChatHistory, h.history, ws.HandlerOptions{})
	hub.Handle(protocol.TypeHandoutShow, h.handout, ws.HandlerOptions{RatePerSec: 2})
}

type handlers struct {
	s      *Service
	roller dice.Roller
}

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

// participant requires the author of a command to be a participant of the campaign.
func participant(cmd ws.Command) error {
	if cmd.Viewer.PlayerID == "" {
		return ws.Errorf(protocol.CodeForbidden, "only a participant of the campaign can do this")
	}
	return nil
}

// commandError turns a service error into the reply of the command.
func commandError(err error) error {
	var perr *dice.ParseError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &perr):
		return ws.Errorf(protocol.CodeBadRequest, "%s", perr.Error())
	case errors.Is(err, ErrNotFound):
		return ws.Errorf(protocol.CodeNotFound, "not found")
	case errors.Is(err, ErrInvalid):
		return ws.Errorf(protocol.CodeBadRequest, "%s", err.Error())
	}
	return err // an unexpected failure: the dispatcher answers internal
}

// message is the renderer of a chat.message; m is captured and never changed.
func message(m protocol.ChatMessage) ws.Renderer {
	return func(v ws.Viewer) (string, any, bool) {
		out, ok := Render(m, v)
		return protocol.TypeChatMessage, out, ok
	}
}

func (h *handlers) send(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	if err := participant(cmd); err != nil {
		return nil, err
	}
	var in protocol.ChatSend
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	m, err := h.s.SendText(ctx, cmd.CampaignID, cmd.GameSessionID, cmd.Viewer.PlayerID, in.Text, in.To)
	if err != nil {
		return nil, commandError(err)
	}
	em.Emit(message(m))
	return nil, nil
}

func (h *handlers) handout(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	if !cmd.Viewer.IsGM {
		return nil, ws.Errorf(protocol.CodeForbidden, "only the GM can show a handout")
	}
	var in protocol.HandoutShow
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	m, err := h.s.SendHandout(ctx, cmd.CampaignID, cmd.GameSessionID, cmd.Viewer.PlayerID, in.Title, in.Content, in.To)
	if err != nil {
		return nil, commandError(err)
	}
	em.Emit(message(m))
	return nil, nil
}

func (h *handlers) roll(ctx context.Context, cmd ws.Command, em ws.Emitter) (any, error) {
	if err := participant(cmd); err != nil {
		return nil, err
	}
	var in protocol.RollRequest
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	m, err := h.s.SendRoll(ctx, cmd.CampaignID, cmd.GameSessionID, cmd.Viewer.PlayerID, in.Formula, in.Visibility, in.Label, h.roller)
	if err != nil {
		return nil, commandError(err)
	}
	em.Emit(message(m))
	return nil, nil
}

func (h *handlers) history(ctx context.Context, cmd ws.Command, _ ws.Emitter) (any, error) {
	if err := participant(cmd); err != nil {
		return nil, err
	}
	var in protocol.ChatHistory
	if err := decode(cmd, &in); err != nil {
		return nil, err
	}
	msgs, err := h.s.History(ctx, cmd.Viewer, cmd.CampaignID, in.Before, in.Limit)
	if err != nil {
		return nil, commandError(err)
	}
	return protocol.ChatHistoryAck{Messages: msgs}, nil
}
