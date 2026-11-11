package project

import "testing"

// Lines a fortieth of the strip apart, so a paragraph's foot is easy to read.
func linesEvery(n int, text func(i int) string) []Line {
	out := make([]Line, n)
	for i := range out {
		out[i] = Line{Number: i + 1, Start: float64(i) / 40, Text: text(i)}
	}
	return out
}

func TestParagraphBottomIsALineBelowTheLast(t *testing.T) {
	// Two paragraphs of three lines: an indented opener, then continuations.
	r := &Reading{Lines: linesEvery(6, func(i int) string {
		if i%3 == 0 {
			return "     opens the paragraph"
		}
		return "continues it"
	})}

	ps := r.Paragraphs()
	if len(ps) != 2 {
		t.Fatalf("got %d paragraphs, want 2", len(ps))
	}
	// The first runs over lines 0..2; its foot is where line 3 begins.
	if ps[0].End != 2.0/40 {
		t.Fatalf("End %v, want the top of its last line", ps[0].End)
	}
	if ps[0].Bottom != 3.0/40 {
		t.Fatalf("Bottom %v, want the top of the next line — otherwise the "+
			"last line is framed with its descenders cut", ps[0].Bottom)
	}
	if ps[0].Height() != 3.0/40 {
		t.Fatalf("Height %v, want three lines", ps[0].Height())
	}
}

// The last paragraph has nothing after it to measure against.
func TestLastParagraphStillHasAFoot(t *testing.T) {
	r := &Reading{Lines: linesEvery(3, func(i int) string {
		if i == 0 {
			return "     opens"
		}
		return "continues"
	})}

	ps := r.Paragraphs()
	last := ps[len(ps)-1]
	if last.Bottom <= last.End {
		t.Fatalf("Bottom %v must sit below End %v", last.Bottom, last.End)
	}
}

// pdftotext marks a page break with a form feed on the first line of the page.
// Counting only spaces as indentation hid those paragraph openings, and the
// server's split then drifted out of step with the console's.
func TestParagraphsSeeThroughAFormFeed(t *testing.T) {
	r := &Reading{Lines: []Line{
		{Number: 1, Start: 0.00, Text: "     opens the first paragraph"},
		{Number: 2, Start: 0.01, Text: "and continues"},
		{Number: 3, Start: 0.02, Text: "\x0c     opens the second, across a page break"},
		{Number: 4, Start: 0.03, Text: "and continues"},
	}}

	ps := r.Paragraphs()
	if len(ps) != 2 {
		t.Fatalf("got %d paragraphs, want 2 — the form feed hid an opening", len(ps))
	}
	if ps[1].From != 2 {
		t.Fatalf("second paragraph starts at line index %d, want 2", ps[1].From)
	}
}
