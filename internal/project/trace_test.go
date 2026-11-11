package project

import (
	"math"
	"testing"
)

func traced() Take {
	// A take that scrolled evenly from 10% to 30% of the strip over 100 s.
	var points []TracePoint
	for i := 0; i <= 100; i += 10 {
		points = append(points, TracePoint{At: float64(i), F: 0.10 + 0.002*float64(i)})
	}
	return Take{Duration: 100, Trace: points}
}

func TestPositionAtInterpolates(t *testing.T) {
	take := traced()
	cases := []struct {
		seconds float64
		want    float64
	}{
		{0, 0.10},
		{50, 0.20},
		{100, 0.30},
		{55, 0.21}, // between samples
		{5, 0.11},
	}
	for _, c := range cases {
		got, ok := take.PositionAt(c.seconds)
		if !ok {
			t.Fatalf("no trace at %.0fs", c.seconds)
		}
		if math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("at %.0fs got %.5f, want %.5f", c.seconds, got, c.want)
		}
	}
}

func TestPositionAtClampsOutsideTheTake(t *testing.T) {
	take := traced()
	if got, _ := take.PositionAt(-30); math.Abs(got-0.10) > 1e-9 {
		t.Fatalf("before the start got %.5f, want 0.10", got)
	}
	if got, _ := take.PositionAt(999); math.Abs(got-0.30) > 1e-9 {
		t.Fatalf("after the end got %.5f, want 0.30", got)
	}
}

func TestPositionAtWithoutTraceReportsMissing(t *testing.T) {
	// Takes recorded before the trace existed must say so rather than lie.
	if _, ok := (Take{Duration: 10}).PositionAt(5); ok {
		t.Fatal("a take with no trace claimed a position")
	}
}

func TestLineAtFindsTheLastLineAtOrBefore(t *testing.T) {
	r := &Reading{Lines: []Line{
		{Number: 1, Start: 0.00, Text: "uno"},
		{Number: 2, Start: 0.25, Text: "dos"},
		{Number: 3, Start: 0.50, Text: "tres"},
		{Number: 4, Start: 0.75, Text: "cuatro"},
	}}
	cases := []struct {
		f    float64
		want int
	}{
		{0.00, 1}, {0.10, 1}, {0.25, 2}, {0.49, 2},
		{0.50, 3}, {0.74, 3}, {0.75, 4}, {1.00, 4},
	}
	for _, c := range cases {
		line, ok := r.LineAt(c.f)
		if !ok || line.Number != c.want {
			t.Fatalf("at %.2f got line %d, want %d", c.f, line.Number, c.want)
		}
	}
}

func TestLineAtOnEmptyReading(t *testing.T) {
	if _, ok := (&Reading{}).LineAt(0.5); ok {
		t.Fatal("an empty reading claimed a line")
	}
}

func TestCutMapsToLinesThroughTheTrace(t *testing.T) {
	// The point of the trace: a cut in seconds becomes a stretch of the passage.
	take := traced()
	r := &Reading{Lines: []Line{
		{Number: 10, Start: 0.10}, {Number: 20, Start: 0.18},
		{Number: 30, Start: 0.24}, {Number: 40, Start: 0.30},
	}}

	from, _ := take.PositionAt(40) // 0.18
	to, _ := take.PositionAt(70)   // 0.24

	a, _ := r.LineAt(from)
	b, _ := r.LineAt(to)
	if a.Number != 20 || b.Number != 30 {
		t.Fatalf("cut 40s-70s mapped to lines %d-%d, want 20-30", a.Number, b.Number)
	}
}
