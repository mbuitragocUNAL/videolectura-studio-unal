package project

import "testing"

// A paragraph can name two figures, each shown with its caption as a take of
// its own. Keyed on the paragraph and the plate flag alone, the second figure's
// take replaced the first's and one figure vanished from the video.
func TestOrderedTakesKeepsOneTakePerFigure(t *testing.T) {
	sess := Session{Takes: []Take{
		{ID: 1, Paragraph: 5, Duration: 5},
		{ID: 2, Paragraph: 5, Plate: true, Figure: "figura-27", From: 0.5, Duration: 4},
		{ID: 3, Paragraph: 5, Plate: true, Figure: "figura-28", From: 0.5, Duration: 4},
	}}

	got := sess.OrderedTakes()
	want := []int{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("got %d takes, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("take %d of the render is %d, want %d", i, got[i].ID, id)
		}
	}
}

// A figure goes after the paragraph whose sentence names it, so a mention has
// to resolve to the paragraph holding its line.
func TestParagraphOfLine(t *testing.T) {
	r := &Reading{Lines: linesEvery(6, func(i int) string {
		if i%3 == 0 {
			return "     opens the paragraph"
		}
		return "continues it"
	})}

	for line, want := range map[int]int{1: 0, 3: 0, 4: 1, 6: 1, 9: -1} {
		if got := r.ParagraphOfLine(line); got != want {
			t.Errorf("line %d is in paragraph %d, want %d", line, got, want)
		}
	}
}
