package vtt

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// points builds the JSON of n points.
func points(n int) string {
	p := make([]string, n)
	for i := range p {
		p[i] = "[1,2]" // 2000 of them still fit in 16 KiB
	}
	return `{"points":[` + strings.Join(p, ",") + `]}`
}

func text(n int) string {
	return fmt.Sprintf(`{"x":1,"y":2,"text":%q,"size":16}`, strings.Repeat("a", n))
}

func TestValidDrawingData(t *testing.T) {
	cases := []struct {
		name string
		kind string
		data string
		want string // the normalized JSON; "" means an error
	}{
		{"freehand", "freehand", `{ "points": [[1,2], [3.5,4]] }`, `{"points":[[1,2],[3.5,4]]}`},
		{"freehand one point", "freehand", points(1), ""},
		{"freehand two points", "freehand", points(2), points(2)},
		{"freehand 2000 points", "freehand", points(2000), points(2000)},
		{"freehand 2001 points", "freehand", points(2001), ""},
		{"freehand point of three numbers", "freehand", `{"points":[[1,2,3],[3,4]]}`, ""},
		{"freehand point of one number", "freehand", `{"points":[[1],[3,4]]}`, ""},
		{"freehand point out of range", "freehand", `{"points":[[1,2],[1e300,4]]}`, ""},
		{"freehand null", "freehand", `null`, ""},

		{"rect", "rect", `{"h":4,"w":3,"y":2,"x":1}`, `{"x":1,"y":2,"w":3,"h":4}`},
		{"rect zero width", "rect", `{"x":1,"y":2,"w":0,"h":4}`, ""},
		{"rect zero height", "rect", `{"x":1,"y":2,"w":3,"h":0}`, ""},
		{"rect negative width", "rect", `{"x":1,"y":2,"w":-3,"h":4}`, ""},
		{"rect at the limit", "rect", `{"x":-100000,"y":100000,"w":1,"h":1}`, `{"x":-100000,"y":100000,"w":1,"h":1}`},
		{"rect past the limit", "rect", `{"x":100000.5,"y":0,"w":1,"h":1}`, ""},
		{"rect far past the limit", "rect", `{"x":1e300,"y":0,"w":1,"h":1}`, ""},
		{"rect number too big for float64", "rect", `{"x":1e999,"y":0,"w":1,"h":1}`, ""},
		{"rect extra field", "rect", `{"x":1,"y":2,"w":3,"h":4,"color":"#ffffff"}`, ""},
		{"rect missing field", "rect", `{"x":1,"y":2,"w":3}`, ""},
		{"rect trailing data", "rect", `{"x":1,"y":2,"w":3,"h":4} {}`, ""},
		{"rect not an object", "rect", `[1,2,3,4]`, ""},
		{"rect empty", "rect", ``, ""},

		{"circle", "circle", `{"r":5,"y":2,"x":1}`, `{"x":1,"y":2,"r":5}`},
		{"circle zero radius", "circle", `{"x":1,"y":2,"r":0}`, ""},
		{"circle huge radius", "circle", `{"x":1,"y":2,"r":1e300}`, ""},
		{"circle extra field", "circle", `{"x":1,"y":2,"r":5,"w":1}`, ""},

		{"text", "text", `{"size":16,"text":"Ловушка","y":2,"x":1}`, `{"x":1,"y":2,"text":"Ловушка","size":16}`},
		{"text empty", "text", text(0), ""},
		{"text one character", "text", text(1), text(1)},
		{"text 500 characters", "text", text(500), text(500)},
		{"text 501 characters", "text", text(501), ""},
		{"text counts characters, not bytes", "text", fmt.Sprintf(`{"x":1,"y":2,"text":%q,"size":16}`, strings.Repeat("ы", 500)),
			fmt.Sprintf(`{"x":1,"y":2,"text":%q,"size":16}`, strings.Repeat("ы", 500))},
		{"text size 7", "text", `{"x":1,"y":2,"text":"a","size":7}`, ""},
		{"text size 8", "text", `{"x":1,"y":2,"text":"a","size":8}`, `{"x":1,"y":2,"text":"a","size":8}`},
		{"text size 200", "text", `{"x":1,"y":2,"text":"a","size":200}`, `{"x":1,"y":2,"text":"a","size":200}`},
		{"text size 201", "text", `{"x":1,"y":2,"text":"a","size":201}`, ""},
		{"text with points", "text", `{"x":1,"y":2,"text":"a","size":16,"points":[[1,2],[3,4]]}`, ""},

		{"template", "template", `{"direction":45,"size":15,"y":2,"x":1,"shape":"cone"}`, `{"shape":"cone","x":1,"y":2,"size":15,"direction":45}`},
		{"template size 4", "template", `{"shape":"cube","x":1,"y":2,"size":4,"direction":0}`, ""},
		{"template size 5", "template", `{"shape":"cube","x":1,"y":2,"size":5,"direction":0}`, `{"shape":"cube","x":1,"y":2,"size":5,"direction":0}`},
		{"template size 1000", "template", `{"shape":"line","x":1,"y":2,"size":1000,"direction":0}`, `{"shape":"line","x":1,"y":2,"size":1000,"direction":0}`},
		{"template size 1001", "template", `{"shape":"line","x":1,"y":2,"size":1001,"direction":0}`, ""},
		{"template unknown shape", "template", `{"shape":"wall","x":1,"y":2,"size":10,"direction":0}`, ""},
		{"template sphere", "template", `{"shape":"sphere","x":1,"y":2,"size":20,"direction":0}`, `{"shape":"sphere","x":1,"y":2,"size":20,"direction":0}`},
		{"template cylinder", "template", `{"shape":"cylinder","x":1,"y":2,"size":20,"direction":0}`, `{"shape":"cylinder","x":1,"y":2,"size":20,"direction":0}`},
		{"template direction -90", "template", `{"shape":"cone","x":1,"y":2,"size":15,"direction":-90}`, `{"shape":"cone","x":1,"y":2,"size":15,"direction":270}`},
		{"template direction 360", "template", `{"shape":"cone","x":1,"y":2,"size":15,"direction":360}`, `{"shape":"cone","x":1,"y":2,"size":15,"direction":0}`},
		{"template direction 725", "template", `{"shape":"cone","x":1,"y":2,"size":15,"direction":725}`, `{"shape":"cone","x":1,"y":2,"size":15,"direction":5}`},
		{"template x out of range", "template", `{"shape":"cone","x":1e300,"y":2,"size":15,"direction":0}`, ""},
		{"template extra field", "template", `{"shape":"cone","x":1,"y":2,"size":15,"direction":0,"color":"#ffffff"}`, ""},
		{"template missing shape", "template", `{"x":1,"y":2,"size":15,"direction":0}`, ""},

		{"unknown kind", "arrow", `{"x":1,"y":2,"w":3,"h":4}`, ""},
		{"empty kind", "", `{"x":1,"y":2,"w":3,"h":4}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validDrawingData(tc.kind, []byte(tc.data))
			if tc.want == "" {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("got %s, %v, want ErrInvalid", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestDrawingDataSizeLimit(t *testing.T) {
	// A freehand that is valid by the points but not by the bytes.
	pad := func(total int) string {
		base := `{"points":[[1,2],[3,4]]}`
		return strings.TrimSuffix(base, "}") + strings.Repeat(" ", total-len(base)) + "}"
	}
	if got, err := validDrawingData("freehand", []byte(pad(16<<10))); err != nil {
		t.Fatalf("16 KiB: %s, %v", got, err)
	}
	if _, err := validDrawingData("freehand", []byte(pad(16<<10+1))); !errors.Is(err, ErrInvalid) {
		t.Fatalf("16 KiB + 1 byte: %v, want ErrInvalid", err)
	}
}

func TestDrawingDataIsNormalized(t *testing.T) {
	// Whitespace and the order of the keys do not reach the storage; a repeated
	// key keeps the last value and is written once.
	got, err := validDrawingData("circle", []byte("{\n \"r\": 5, \"x\": 1, \"y\": 2, \"x\": 9 }"))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"x":9,"y":2,"r":5}`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
