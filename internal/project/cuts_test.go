package project

import (
	"math"
	"testing"
)

func TestMergeCuts(t *testing.T) {
	cases := []struct {
		name string
		in   []Cut
		want []Cut
	}{
		{"sorts", []Cut{{5, 7}, {1, 2}}, []Cut{{1, 2}, {5, 7}}},
		{"merges overlapping", []Cut{{1, 5}, {3, 8}}, []Cut{{1, 8}}},
		{"merges touching", []Cut{{1, 3}, {3, 6}}, []Cut{{1, 6}}},
		{"drops empty", []Cut{{4, 4}}, nil},
		{"drops inverted", []Cut{{9, 4}}, nil},
		{"keeps disjoint", []Cut{{1, 2}, {5, 7}}, []Cut{{1, 2}, {5, 7}}},
		{"swallows contained", []Cut{{1, 10}, {3, 4}}, []Cut{{1, 10}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := MergeCuts(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
}

func TestKeptSpans(t *testing.T) {
	cases := []struct {
		name string
		cuts []Cut
		end  float64
		want []Span
	}{
		{"cut in the middle", []Cut{{10, 20}}, 60, []Span{{0, 10}, {20, 60}}},
		{"cut at the start", []Cut{{0, 5}}, 60, []Span{{5, 60}}},
		{"cut at the end", []Cut{{50, 60}}, 60, []Span{{0, 50}}},
		{"two cuts", []Cut{{10, 15}, {30, 35}}, 60, []Span{{0, 10}, {15, 30}, {35, 60}}},
		{"no cuts", nil, 60, []Span{{0, 60}}},
		{"cut past the end is clamped", []Cut{{50, 90}}, 60, []Span{{0, 50}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := KeptSpans(c.cuts, c.end)
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
}

func TestKeptSpansSumIsRenderedDuration(t *testing.T) {
	cuts := []Cut{{10, 15}, {30, 35}}
	var total float64
	for _, s := range KeptSpans(cuts, 60) {
		total += s.Length()
	}
	if math.Abs(total-50) > 1e-9 {
		t.Fatalf("kept %.3fs, want 50s", total)
	}
}

func TestRetimeCues(t *testing.T) {
	cues := []Cue{{"a", 5}, {"b", 12}, {"c", 25}, {"d", 40}}
	kept, dropped := RetimeCues(cues, []Cut{{10, 20}})

	if len(dropped) != 1 || dropped[0].Quote != "b" {
		t.Fatalf("expected b to be dropped, got %v", dropped)
	}
	want := map[string]float64{"a": 5, "c": 15, "d": 30}
	if len(kept) != len(want) {
		t.Fatalf("kept %v", kept)
	}
	for _, c := range kept {
		if math.Abs(c.At-want[c.Quote]) > 1e-9 {
			t.Fatalf("%s at %.3f, want %.3f", c.Quote, c.At, want[c.Quote])
		}
	}
}

func TestRetimeCuesAccumulatesCuts(t *testing.T) {
	kept, _ := RetimeCues([]Cue{{"x", 50}}, []Cut{{5, 10}, {20, 26}})
	if len(kept) != 1 || math.Abs(kept[0].At-39) > 1e-9 {
		t.Fatalf("got %v, want 39s", kept)
	}
}

func TestRetimeCuesWithoutCutsIsIdentity(t *testing.T) {
	cues := []Cue{{"a", 5}, {"b", 12}}
	kept, dropped := RetimeCues(cues, nil)
	if len(dropped) != 0 || len(kept) != 2 || kept[0].At != 5 || kept[1].At != 12 {
		t.Fatalf("got %v / %v", kept, dropped)
	}
}

func TestKeptDuration(t *testing.T) {
	take := Take{Duration: 300, Cuts: []Cut{{10, 20}, {100, 130}}}
	if got := take.KeptDuration(); math.Abs(got-260) > 1e-9 {
		t.Fatalf("got %.3f, want 260", got)
	}
}

func TestKeptDurationIgnoresOverlap(t *testing.T) {
	// Overlapping cuts must not be counted twice.
	take := Take{Duration: 100, Cuts: []Cut{{10, 30}, {20, 40}}}
	if got := take.KeptDuration(); math.Abs(got-70) > 1e-9 {
		t.Fatalf("got %.3f, want 70", got)
	}
}

func TestNextTakeIDSurvivesDeletion(t *testing.T) {
	s := Session{Takes: []Take{{ID: 1}, {ID: 4}}}
	if got := s.NextTakeID(); got != 5 {
		t.Fatalf("got %d, want 5", got)
	}
}

// Takes are assembled one per paragraph, in the passage's order.
func TestOrderedTakesIsOnePerParagraphInPassageOrder(t *testing.T) {
	s := Session{Takes: []Take{
		{ID: 1, Paragraph: 2, Duration: 5},
		{ID: 2, Paragraph: 0, Duration: 5},
		{ID: 3, Paragraph: 1, Duration: 5},
	}}
	got := s.OrderedTakes()
	for i, want := range []int{2, 3, 1} {
		if got[i].ID != want {
			t.Fatalf("take %d is %d, want %d", i, got[i].ID, want)
		}
	}
}

func TestOrderedTakesHonoursTimeline(t *testing.T) {
	s := Session{
		Takes: []Take{{ID: 1, Duration: 5}, {ID: 2, Duration: 5}, {ID: 3, Duration: 5}},
		Timeline: Timeline{Entries: []Entry{
			{TakeID: 3}, {TakeID: 1, Muted: true}, {TakeID: 2}, {TakeID: 99},
		}},
	}
	got := s.OrderedTakes()
	if len(got) != 2 || got[0].ID != 3 || got[1].ID != 2 {
		t.Fatalf("got %v", got)
	}
}

// A remnant too short to be faded is a click, not a fragment of the reading.
func TestKeptSpansDropsSliversTooShortToFade(t *testing.T) {
	// 7.17 of a 7.20 second take cut away: the 30 ms left is not speech.
	spans := KeptSpans([]Cut{{From: 0, To: 7.17}}, 7.20)
	if len(spans) != 0 {
		t.Fatalf("kept %v; want nothing", spans)
	}

	// A tenth of a second still survives: it can be faded, so it is real.
	spans = KeptSpans([]Cut{{From: 0, To: 7.1}}, 7.20)
	if len(spans) != 1 {
		t.Fatalf("kept %v; want one span", spans)
	}
}
