package vtt

import (
	"math"
	"strings"
	"testing"

	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

func rect(x, y, w, h float64) protocol.Shape {
	return protocol.Shape{Type: protocol.ShapeRect, X: x, Y: y, W: w, H: h}
}

func circle(x, y, r float64) protocol.Shape {
	return protocol.Shape{Type: protocol.ShapeCircle, X: x, Y: y, R: r}
}

func polygon(pts ...[2]float64) protocol.Shape {
	return protocol.Shape{Type: protocol.ShapePolygon, Points: pts}
}

func TestValidShape(t *testing.T) {
	many := func(n int) protocol.Shape {
		pts := make([][2]float64, n)
		for i := range pts {
			pts[i] = [2]float64{float64(i), float64(i % 5)}
		}
		return polygon(pts...)
	}
	tests := []struct {
		name string
		s    protocol.Shape
		want bool
	}{
		{"rect", rect(0, 0, 10, 10), true},
		{"rect w = 0", rect(0, 0, 0, 10), false},
		{"rect h = 0", rect(0, 0, 10, 0), false},
		{"rect w < 0", rect(0, 0, -1, 10), false},
		{"rect at the limit", rect(-maxCoord, maxCoord, 1, 1), true},
		{"rect over the limit", rect(maxCoord+1, 0, 1, 1), false},
		{"rect NaN", rect(math.NaN(), 0, 1, 1), false},
		{"rect Inf w", rect(0, 0, math.Inf(1), 1), false},
		{"circle", circle(5, 5, 1), true},
		{"circle r = 0", circle(5, 5, 0), false},
		{"circle r < 0", circle(5, 5, -2), false},
		{"circle Inf", circle(math.Inf(-1), 0, 1), false},
		{"circle over the limit", circle(0, 100001, 1), false},
		{"polygon of 2", many(2), false},
		{"polygon of 3", many(3), true},
		{"polygon of 200", many(maxPolygonPoints), true},
		{"polygon of 201", many(maxPolygonPoints + 1), false},
		{"polygon with NaN", polygon([2]float64{0, 0}, [2]float64{1, 0}, [2]float64{math.NaN(), 1}), false},
		{"polygon with 100001", polygon([2]float64{0, 0}, [2]float64{1, 0}, [2]float64{100001, 1}), false},
		{"empty type", protocol.Shape{}, false},
		{"unknown type", protocol.Shape{Type: "star", X: 1, Y: 1, R: 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validShape(tt.s); got != tt.want {
				t.Errorf("validShape = %t, want %t", got, tt.want)
			}
		})
	}
}

type pt struct {
	name string
	x, y float64
	want bool
}

func checkContains(t *testing.T, s protocol.Shape, points []pt) {
	t.Helper()
	for _, p := range points {
		if got := contains(s, p.x, p.y); got != p.want {
			t.Errorf("%s (%v, %v): contains = %t, want %t", p.name, p.x, p.y, got, p.want)
		}
	}
}

func TestContainsRect(t *testing.T) {
	checkContains(t, rect(10, 20, 30, 40), []pt{
		{"inside", 20, 30, true},
		{"left side", 10, 30, true},
		{"right side", 40, 30, true},
		{"top side", 20, 20, true},
		{"bottom side", 20, 60, true},
		{"top-left corner", 10, 20, true},
		{"bottom-right corner", 40, 60, true},
		{"outside left", 10 - 1e-6, 30, false},
		{"outside right", 40 + 1e-6, 30, false},
		{"outside top", 20, 20 - 1e-6, false},
		{"outside bottom", 20, 60 + 1e-6, false},
		{"far away", 1000, 1000, false},
	})
}

func TestContainsCircle(t *testing.T) {
	checkContains(t, circle(100, 100, 50), []pt{
		{"center", 100, 100, true},
		{"on the circle, east", 150, 100, true},
		{"on the circle, north", 100, 50, true},
		{"on the circle, 3-4-5", 130, 140, true},
		{"outside", 150 + 1e-6, 100, false},
		{"far away", 0, 0, false},
	})
}

func TestContainsPolygon(t *testing.T) {
	// A concave "П": the legs go down, the notch between them is outside.
	pi := polygon(
		[2]float64{0, 0}, [2]float64{30, 0}, [2]float64{30, 30}, [2]float64{20, 30},
		[2]float64{20, 10}, [2]float64{10, 10}, [2]float64{10, 30}, [2]float64{0, 30},
	)
	checkContains(t, pi, []pt{
		{"in the bar", 15, 5, true},
		{"in the left leg", 5, 20, true},
		{"in the right leg", 25, 20, true},
		{"in the notch", 15, 20, false},
		{"below the notch", 15, 40, false},
		{"left of the shape", -1, 5, false},
		{"on the outer edge", 0, 15, true},
		{"on a notch edge", 10, 20, true},
		{"on the notch top edge", 15, 10, true},
		{"at a vertex", 20, 10, true},
		{"at an outer vertex", 30, 30, true},
	})

	// A bow tie: the two lobes are inside, the crossing point is on both edges.
	bow := polygon([2]float64{0, 0}, [2]float64{20, 20}, [2]float64{20, 0}, [2]float64{0, 20})
	checkContains(t, bow, []pt{
		{"left lobe", 3, 10, true},
		{"right lobe", 17, 10, true},
		{"above the crossing", 10, 3, false},
		{"below the crossing", 10, 17, false},
		{"the crossing", 10, 10, true},
	})

	tri := polygon([2]float64{0, 0}, [2]float64{10, 0}, [2]float64{0, 10})
	checkContains(t, tri, []pt{
		{"on the hypotenuse", 5, 5, true},
		{"just outside the hypotenuse", 5.001, 5.001, false},
		{"on the horizontal edge", 4, 0, true},
		{"on the vertical edge", 0, 7, true},
		{"on the line of an edge, past its end", 11, 0, false},
	})
}

func TestRevealed(t *testing.T) {
	a := rect(0, 0, 100, 100)
	b := rect(40, 40, 20, 20)
	op := func(kind string, s protocol.Shape) protocol.FogOp { return protocol.FogOp{Op: kind, Shape: s} }

	t.Run("fog off reveals everything", func(t *testing.T) {
		f := protocol.Fog{Enabled: false, Ops: []protocol.FogOp{op("hide", a)}}
		if !revealed(f, 50, 50) || !revealed(f, 5000, 5000) {
			t.Error("a disabled fog hid a point")
		}
	})
	t.Run("no ops hide everything", func(t *testing.T) {
		if revealed(protocol.Fog{Enabled: true}, 50, 50) {
			t.Error("a point is revealed with no operations")
		}
	})
	t.Run("the last operation decides", func(t *testing.T) {
		steps := []struct {
			name         string
			ops          []protocol.FogOp
			inB, inAOnly bool
		}{
			{"reveal A", []protocol.FogOp{op("reveal", a)}, true, true},
			{"reveal A, hide B", []protocol.FogOp{op("reveal", a), op("hide", b)}, false, true},
			{"reveal A, hide B, reveal B", []protocol.FogOp{op("reveal", a), op("hide", b), op("reveal", b)}, true, true},
			{"hide B, reveal A", []protocol.FogOp{op("hide", b), op("reveal", a)}, true, true},
		}
		for _, st := range steps {
			f := protocol.Fog{Enabled: true, Ops: st.ops}
			if got := revealed(f, 50, 50); got != st.inB {
				t.Errorf("%s: point in B = %t, want %t", st.name, got, st.inB)
			}
			if got := revealed(f, 10, 10); got != st.inAOnly {
				t.Errorf("%s: point in A outside B = %t, want %t", st.name, got, st.inAOnly)
			}
			if revealed(f, 500, 500) {
				t.Errorf("%s: a point in no shape is revealed", st.name)
			}
		}
	})
	t.Run("the border of a shape counts", func(t *testing.T) {
		f := protocol.Fog{Enabled: true, Ops: []protocol.FogOp{op("reveal", a)}}
		if !revealed(f, 100, 100) {
			t.Error("the corner of a revealed rectangle is hidden")
		}
	})
}

func TestFogOpsNeverNullInJSON(t *testing.T) {
	raw, err := (protocol.Fog{Enabled: true}).MarshalJSON()
	if err != nil || !strings.Contains(string(raw), `"ops":[]`) {
		t.Errorf("got %s, %v", raw, err)
	}
}
