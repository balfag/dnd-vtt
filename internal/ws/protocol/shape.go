package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Shape types.
const (
	ShapeRect    = "rect"
	ShapeCircle  = "circle"
	ShapePolygon = "polygon"
)

// Shape is one of {"type":"rect","x","y","w","h"}, {"type":"circle","x","y","r"},
// {"type":"polygon","points":[[x,y],…]}. Zero coordinates are encoded (no omitempty).
type Shape struct {
	Type   string       // rect | circle | polygon
	X, Y   float64      // rect: top-left corner; circle: center
	W, H   float64      // rect
	R      float64      // circle
	Points [][2]float64 // polygon
}

// shapeFields lists the fields a shape of each type has, besides "type".
var shapeFields = map[string][]string{
	ShapeRect:    {"x", "y", "w", "h"},
	ShapeCircle:  {"x", "y", "r"},
	ShapePolygon: {"points"},
}

// MarshalJSON encodes only the fields of s.Type.
func (s Shape) MarshalJSON() ([]byte, error) {
	switch s.Type {
	case ShapeRect:
		return json.Marshal(struct {
			Type string  `json:"type"`
			X    float64 `json:"x"`
			Y    float64 `json:"y"`
			W    float64 `json:"w"`
			H    float64 `json:"h"`
		}{s.Type, s.X, s.Y, s.W, s.H})
	case ShapeCircle:
		return json.Marshal(struct {
			Type string  `json:"type"`
			X    float64 `json:"x"`
			Y    float64 `json:"y"`
			R    float64 `json:"r"`
		}{s.Type, s.X, s.Y, s.R})
	case ShapePolygon:
		points := s.Points
		if points == nil {
			points = [][2]float64{}
		}
		return json.Marshal(struct {
			Type   string       `json:"type"`
			Points [][2]float64 `json:"points"`
		}{s.Type, points})
	}
	return nil, fmt.Errorf("unknown shape type %q", s.Type)
}

// UnmarshalJSON fails on an unknown type and on extra or missing fields.
func (s *Shape) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil || raw == nil {
		return errors.New("a shape must be an object")
	}
	var typ string
	if err := json.Unmarshal(raw["type"], &typ); err != nil {
		return errors.New("a shape needs a string type")
	}
	fields, ok := shapeFields[typ]
	if !ok {
		return fmt.Errorf("unknown shape type %q", typ)
	}
	if len(raw) != len(fields)+1 {
		return fmt.Errorf("a %s has exactly the fields type and %v", typ, fields)
	}
	for _, f := range fields {
		if _, ok := raw[f]; !ok {
			return fmt.Errorf("a %s needs the field %q", typ, f)
		}
	}

	out := Shape{Type: typ}
	var err error
	switch typ {
	case ShapeRect:
		err = numbers(raw, map[string]*float64{"x": &out.X, "y": &out.Y, "w": &out.W, "h": &out.H})
	case ShapeCircle:
		err = numbers(raw, map[string]*float64{"x": &out.X, "y": &out.Y, "r": &out.R})
	case ShapePolygon:
		var pairs [][]float64
		if err = json.Unmarshal(raw["points"], &pairs); err != nil || pairs == nil {
			return errors.New("points must be an array of [x, y]")
		}
		out.Points = make([][2]float64, len(pairs))
		for i, p := range pairs {
			if len(p) != 2 {
				return errors.New("points must be an array of [x, y]")
			}
			out.Points[i] = [2]float64{p[0], p[1]}
		}
	}
	if err != nil {
		return err
	}
	*s = out
	return nil
}

// numbers decodes the named fields of raw, which must be JSON numbers.
func numbers(raw map[string]json.RawMessage, dst map[string]*float64) error {
	for name, p := range dst {
		if err := json.Unmarshal(raw[name], p); err != nil || string(raw[name]) == "null" {
			return fmt.Errorf("%q must be a number", name)
		}
	}
	return nil
}
