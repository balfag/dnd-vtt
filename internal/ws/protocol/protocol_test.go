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

func intPtr(n int) *int { return &n }

// examples are the reference messages of the protocol, one per file in
// testdata. The TypeScript side reads the same files.
func examples(t *testing.T) map[string]Envelope {
	t.Helper()
	newPlayer := PlayerInfo{ID: "01J00000000000000000000PL3", DisplayName: "Борис", Color: "#43a047", Role: "player", Online: true}
	errReply, err := ErrorReply("c-9", CodeBadRequest, "unknown message type")
	if err != nil {
		t.Fatal(err)
	}
	grid := Grid{Type: "square", SizePx: 70, UnitFt: 5, OffsetX: 0, OffsetY: 0, Color: "#9aa8b3", Visible: true}
	hexGrid := Grid{Type: "hexPointy", SizePx: 80, UnitFt: 5, OffsetX: 12.5, OffsetY: 0, Color: "#c9a34e", Visible: false}
	tavern := SceneView{
		ID: "01J00000000000000000000SC1", Name: "Таверна", Width: 3000, Height: 2000, Grid: grid,
		Background: &AssetRef{AssetID: "01J00000000000000000000AS1", URL: "/assets/01J00000000000000000000AS1"},
		Fog:        Fog{Enabled: false, Ops: json.RawMessage("[]")},
	}
	crypt := SceneView{
		ID: "01J00000000000000000000SC2", Name: "Склеп", Width: 2800, Height: 2100, Grid: hexGrid,
		Background: nil,
		Fog:        Fog{Enabled: true, Ops: json.RawMessage("[]")},
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
				SnapshotTable: SnapshotTable{ActiveSceneID: strPtr(tavern.ID), Scenes: []SceneListItem{}, Scene: &tavern},
			},
		}),
		"snapshot_gm_scenes": mustNew(t, TypeSnapshot, Snapshot{
			Seq: 23,
			Me:  SnapshotMe{AccountID: "01J00000000000000000000AC0", IsGM: true, PlayerID: strPtr("01J00000000000000000000PL1")},
			Game: &SnapshotGame{
				GameSessionID: "01J00000000000000000000GS1",
				StartedAt:     1760530000000,
				Campaign:      CampaignRef{ID: "01J00000000000000000000CA1", Name: "Проклятие Страда"},
				Players: []PlayerInfo{
					{ID: "01J00000000000000000000PL1", DisplayName: "master", Color: "#e53935", Role: "gm", Online: true},
				},
				SnapshotTable: SnapshotTable{
					ActiveSceneID: strPtr(crypt.ID),
					Scenes:        []SceneListItem{{ID: tavern.ID, Name: tavern.Name, Sort: 0}, {ID: crypt.ID, Name: crypt.Name, Sort: 1}},
					Scene:         &crypt,
				},
			},
		}),
		"scene_create_command": with(mustNew(t, TypeSceneCreate, SceneCreate{Name: "Таверна", Width: intPtr(3000), Height: intPtr(2000), Grid: &grid}),
			func(e *Envelope) { e.ID = "c-3" }),
		"scene_update_command": with(mustNew(t, TypeSceneUpdate, SceneUpdate{
			SceneID: tavern.ID, Name: strPtr("Таверна «Дракон»"), BackgroundAssetID: json.RawMessage("null"),
		}), func(e *Envelope) { e.ID = "c-4" }),
		"scenes_changed": with(mustNew(t, TypeScenesChanged, ScenesChanged{
			Scenes: []SceneListItem{{ID: tavern.ID, Name: tavern.Name, Sort: 0}, {ID: crypt.ID, Name: crypt.Name, Sort: 1}},
		}), func(e *Envelope) { e.Seq = 20 }),
		"scene_changed":   with(mustNew(t, TypeSceneChanged, SceneChanged{Scene: tavern}), func(e *Envelope) { e.Seq = 21 }),
		"scene_activated": with(mustNew(t, TypeSceneActivated, SceneActivated{Scene: crypt}), func(e *Envelope) { e.Seq = 22 }),
		"scene_view_ack":  with(mustNew(t, TypeAck, SceneViewAck{Scene: tavern}), func(e *Envelope) { e.Re = "c-5" }),
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
				if name == "scene_view_ack" {
					data = &SceneViewAck{}
				} else {
					data = &HelloAck{}
				}
			case TypeSceneCreate:
				data = &SceneCreate{}
			case TypeSceneUpdate:
				data = &SceneUpdate{}
			case TypeScenesChanged:
				data = &ScenesChanged{}
			case TypeSceneChanged:
				data = &SceneChanged{}
			case TypeSceneActivated:
				data = &SceneActivated{}
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
