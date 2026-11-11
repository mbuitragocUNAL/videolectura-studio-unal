package project

import (
	"fmt"
	"strings"
	"unicode"
)

// Paragraph is a stretch of the passage recorded as one take.
type Paragraph struct {
	Index int
	From  int     // index into Reading.Lines, inclusive
	To    int     // likewise
	Start float64 // top of its first line in the strip, 0..1
	End   float64 // top of its last line
	// Bottom is where the paragraph actually stops, a line below End. Framing
	// on End alone clips the last line, because a line's position is its top.
	Bottom float64
}

// Height is how much of the strip the paragraph occupies.
func (p Paragraph) Height() float64 { return p.Bottom - p.Start }

// Paragraphs cuts the passage where the console cuts it.
//
// This repeats the rule the browser applies, deliberately. The console decides
// which paragraph a take reads and stamps the index on it; the render has to
// turn that index back into a place on the page, and the only way the two can
// agree is by splitting the passage the same way. The rule is small enough to
// state twice and too load-bearing to infer.
//
// pdftotext gives no structure, only leading spaces, and they are not clean: a
// paragraph opens at an indent of four, five or six depending on where the line
// fell, while a continuation of a quoted speech sits at seven.
func (r *Reading) Paragraphs() []Paragraph {
	if len(r.Lines) == 0 {
		return nil
	}

	var starts []int
	previousBlank := true
	for i, line := range r.Lines {
		text := line.Text
		blank := strings.TrimSpace(text) == ""
		if !blank {
			// Every kind of leading whitespace, not just spaces.
			//
			// pdftotext puts a form feed at each page break, so eleven lines
			// of this passage begin with \x0c. Counting only spaces made those
			// lines look unindented, the server split the passage into 77
			// paragraphs where the console makes 79, and from the twentieth
			// onwards every take was framed against the wrong one — the page
			// showed a paragraph the reader was not reading.
			indent := len(text) - len(strings.TrimLeftFunc(text, unicode.IsSpace))
			if previousBlank || (indent >= 3 && indent <= 6) {
				starts = append(starts, i)
			}
		}
		previousBlank = blank
	}

	out := make([]Paragraph, 0, len(starts))
	for n, from := range starts {
		to := len(r.Lines) - 1
		if n+1 < len(starts) {
			to = starts[n+1] - 1
		}
		for to > from && strings.TrimSpace(r.Lines[to].Text) == "" {
			to--
		}
		// The foot of the paragraph is the top of whatever follows it; with
		// nothing after, fall back to the spacing of its own lines.
		bottom := r.Lines[to].Start
		if to+1 < len(r.Lines) {
			bottom = r.Lines[to+1].Start
		} else if to > from {
			bottom += (r.Lines[to].Start - r.Lines[from].Start) / float64(to-from)
		}

		out = append(out, Paragraph{
			Index:  n,
			From:   from,
			To:     to,
			Start:  r.Lines[from].Start,
			End:    r.Lines[to].Start,
			Bottom: bottom,
		})
	}
	return out
}

// MisplacedTakes lists the takes whose label no longer starts like the
// paragraph they point at. A take keeps its paragraph by number, and the
// label is the text that paragraph started with when it was recorded; if
// preparing the reading again changes where paragraphs break, the numbers
// shift and the label is what shows it.
func (r *Reading) MisplacedTakes(takes []Take) []string {
	paragraphs := r.Paragraphs()
	squash := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	var out []string
	for _, t := range takes {
		_, snippet, ok := strings.Cut(t.Label, " · ")
		if t.Plate || !ok {
			continue
		}
		snippet = squash(strings.TrimSuffix(snippet, "…"))
		if t.Paragraph < 0 || t.Paragraph >= len(paragraphs) {
			out = append(out, fmt.Sprintf("toma %d: el párrafo %d ya no existe", t.ID, t.Paragraph+1))
			continue
		}
		p := paragraphs[t.Paragraph]
		var text []string
		for _, l := range r.Lines[p.From : p.To+1] {
			text = append(text, l.Text)
		}
		if !strings.HasPrefix(squash(strings.Join(text, " ")), snippet) {
			out = append(out, fmt.Sprintf("toma %d («%s») apunta al párrafo %d, que ahora empieza de otra forma",
				t.ID, snippet, t.Paragraph+1))
		}
	}
	return out
}
