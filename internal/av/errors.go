package av

import "errors"

var (
	ErrAlreadyJoined = errors.New("av: already in the room")
	ErrRoomClosed    = errors.New("av: room closed")
	ErrBadState      = errors.New("av: unexpected signalling message")
)
