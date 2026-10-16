package protocol

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata")

func mustNew(t *testing.T, typ string, d any) Envelope {
	t.Helper()
	env, err := New(typ, d)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func with(env Envelope, f func(*Envelope)) Envelope {
	f(&env)
	return env
}

func strPtr(s string) *string { return &s }

// examples are the reference messages of the protocol, one per file in
// testdata. The TypeScript side reads the same files.
func examples(t *testing.T) map[string]Envelope {
	t.Helper()
	newPlayer := PlayerInfo{ID: "01J00000000000000000000PL3", DisplayName: "Борис", Color: "#43a047", Role: "player", Online: true}
	errReply, err := ErrorReply("c-9", CodeBadRequest, "unknown message type")
	if err != nil {
		t.Fatal(err)
	}

	return map[string]Envelope{
		"hello_command": with(mustNew(t, TypeHello, Hello{GameSessionID: "01J00000000000000000000GS1", LastSeq: 17}),
			func(e *Envelope) { e.ID = "c-1" }),
		"hello_empty_command": with(mustNew(t, TypeHello, Hello{}),
			func(e *Envelope) { e.ID = "c-1" }),
		"hello_ack_resumed": with(mustNew(t, TypeAck, HelloAck{Resumed: true}),
			func(e *Envelope) { e.Re = "c-1" }),
		"hello_ack_fresh": with(mustNew(t, TypeAck, HelloAck{Resumed: false}),
			func(e *Envelope) { e.Re = "c-1" }),
		"snapshot_player": mustNew(t, TypeSnapshot, Snapshot{
			Seq: 17,
			Me:  SnapshotMe{AccountID: "01J00000000000000000000AC1", IsGM: false, PlayerID: strPtr("01J00000000000000000000PL2")},
			Game: &SnapshotGame{
				GameSessionID: "01J00000000000000000000GS1",
				StartedAt:     1760530000000,
				Campaign:      CampaignRef{ID: "01J00000000000000000000CA1", Name: "Проклятие Страда"},
				Players: []PlayerInfo{
					{ID: "01J00000000000000000000PL1", DisplayName: "master", Color: "#e53935", Role: "gm", Online: true},
					{ID: "01J00000000000000000000PL2", DisplayName: "Анна", Color: "#1e88e5", Role: "player", Online: true},
				},
			},
		}),
		"snapshot_gm_no_game": mustNew(t, TypeSnapshot, Snapshot{
			Seq:  0,
			Me:   SnapshotMe{AccountID: "01J00000000000000000000AC0", IsGM: true, PlayerID: nil},
			Game: nil,
		}),
		"presence_offline": with(mustNew(t, TypePresence, Presence{PlayerID: "01J00000000000000000000PL2", Online: false}),
			func(e *Envelope) { e.Seq = 18 }),
		"presence_new_player": with(mustNew(t, TypePresence, Presence{PlayerID: "01J00000000000000000000PL3", Online: true, Player: &newPlayer}),
			func(e *Envelope) { e.Seq = 19 }),
		"noop":              with(mustNew(t, TypeNoop, nil), func(e *Envelope) { e.Seq = 42 }),
		"ping":              with(mustNew(t, TypePing, nil), func(e *Envelope) { e.ID = "p-5" }),
		"pong":              with(mustNew(t, TypePong, nil), func(e *Envelope) { e.Re = "p-5" }),
		"error_bad_request": errReply,
	}
}

// strictDecode unmarshals data into dst and fails on unknown fields.
func strictDecode(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// TestGolden checks every example against testdata/<name>.json. Run
// `go test ./internal/ws/protocol/ -update` after changing a message on purpose.
func TestGolden(t *testing.T) {
	// os.Root keeps every read and write inside testdata.
	root, err := os.OpenRoot("testdata")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })

	for name, env := range examples(t) {
		t.Run(name, func(t *testing.T) {
			got, err := json.MarshalIndent(env, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')

			file := name + ".json"
			if *update {
				if err := root.WriteFile(file, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := root.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("testdata/%s differs from the example.\n got: %s\nwant: %s\nRun `go test ./internal/ws/protocol/ -update` if the change is intended.", file, got, want)
			}

			// The file must decode back into the matching types, without
			// fields the Go structs do not know.
			var decoded Envelope
			if err := strictDecode(want, &decoded); err != nil {
				t.Fatalf("decode envelope: %v", err)
			}
			if len(decoded.D) == 0 {
				return
			}
			var data any
			switch decoded.T {
			case TypeHello:
				data = &Hello{}
			case TypeAck:
				data = &HelloAck{}
			case TypeSnapshot:
				data = &Snapshot{}
			case TypePresence:
				data = &Presence{}
			case TypeError:
				data = &ErrorData{}
			default:
				t.Fatalf("type %q has data but no Go type in this test", decoded.T)
			}
			if err := strictDecode(decoded.D, data); err != nil {
				t.Errorf("decode data of %s: %v", decoded.T, err)
			}
		})
	}
}
