package protocol

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestShapeRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		shape Shape
		json  string
	}{
		{"rect", Shape{Type: ShapeRect, X: 10, Y: 20.5, W: 30, H: 40}, `{"type":"rect","x":10,"y":20.5,"w":30,"h":40}`},
		{"rect at zero", Shape{Type: ShapeRect, X: 0, Y: 0, W: 5, H: 5}, `{"type":"rect","x":0,"y":0,"w":5,"h":5}`},
		{"circle", Shape{Type: ShapeCircle, X: 1, Y: 2, R: 3}, `{"type":"circle","x":1,"y":2,"r":3}`},
		{"circle at zero", Shape{Type: ShapeCircle, X: 0, Y: 0, R: 7}, `{"type":"circle","x":0,"y":0,"r":7}`},
		{"polygon", Shape{Type: ShapePolygon, Points: [][2]float64{{0, 0}, {10, 0}, {5, 8.5}}}, `{"type":"polygon","points":[[0,0],[10,0],[5,8.5]]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.shape)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.json {
				t.Errorf("marshal = %s, want %s", got, tt.json)
			}
			var back Shape
			if err := json.Unmarshal(got, &back); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(back, tt.shape) {
				t.Errorf("round trip = %+v, want %+v", back, tt.shape)
			}
		})
	}
}

func TestShapeMarshalIgnoresForeignFields(t *testing.T) {
	got, err := json.Marshal(Shape{Type: ShapeCircle, X: 1, Y: 2, R: 3, W: 9, H: 9, Points: [][2]float64{{1, 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"type":"circle","x":1,"y":2,"r":3}` {
		t.Errorf("got %s", got)
	}
	if _, err := json.Marshal(Shape{Type: "star"}); err == nil {
		t.Error("an unknown type was encoded")
	}
}

func TestShapeUnmarshalErrors(t *testing.T) {
	tests := map[string]string{
		"unknown type":         `{"type":"star","x":1}`,
		"no type":              `{"x":1,"y":2,"w":3,"h":4}`,
		"type is not a string": `{"type":1,"x":1,"y":2,"r":3}`,
		"extra field":          `{"type":"rect","x":1,"y":2,"w":3,"h":4,"r":5}`,
		"extra unknown field":  `{"type":"circle","x":1,"y":2,"r":3,"color":"red"}`,
		"missing field":        `{"type":"rect","x":1,"y":2,"w":3}`,
		"missing r":            `{"type":"circle","x":1,"y":2}`,
		"null coordinate":      `{"type":"circle","x":null,"y":2,"r":3}`,
		"string coordinate":    `{"type":"circle","x":"1","y":2,"r":3}`,
		"not an object":        `[1,2]`,
		"null":                 `null`,
		"points is null":       `{"type":"polygon","points":null}`,
		"points not pairs":     `{"type":"polygon","points":[1,2,3]}`,
		"point of three":       `{"type":"polygon","points":[[0,0],[1,1],[2,2,3]]}`,
		"point of one":         `{"type":"polygon","points":[[0,0],[1],[2,2]]}`,
		"point of strings":     `{"type":"polygon","points":[["a","b"],[1,1],[2,2]]}`,
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			var s Shape
			if err := json.Unmarshal([]byte(in), &s); err == nil {
				t.Errorf("accepted %s as %+v", in, s)
			}
		})
	}
}

func TestShapeInsideCommandIsStrict(t *testing.T) {
	var in FogShape
	err := strictDecode([]byte(`{"sceneId":"s","shape":{"type":"rect","x":0,"y":0,"w":1,"h":1,"r":2}}`), &in)
	if err == nil || !strings.Contains(err.Error(), "fields") {
		t.Errorf("err = %v, want a complaint about the fields", err)
	}
}

func TestFogChangedTokensPresence(t *testing.T) {
	fog := Fog{Enabled: true}
	for name, tc := range map[string]struct {
		tokens []Token
		has    bool
	}{"gm": {nil, false}, "player empty": {[]Token{}, true}} {
		raw, err := json.Marshal(FogChanged{SceneID: "s", Fog: fog, Tokens: tc.tokens})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(string(raw), `"tokens":[]`); got != tc.has {
			t.Errorf("%s: %s", name, raw)
		}
		if !strings.Contains(string(raw), `"ops":[]`) {
			t.Errorf("%s: nil ops not encoded as []: %s", name, raw)
		}
	}
}
