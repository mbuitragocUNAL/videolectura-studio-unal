package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

/*
Typeset is the passage set again in the book's own type, written by
preparar/componer.py as tipografia.json beside marcas.json.

The video used to show the book's pages as they are: with their margins, with
a gap at every page break, and with smaller text whenever a figure fixed on
the right took part of the screen. Lineamientos 2.c and 2.f ask for text that
fills the screen and stays one size with or without a figure, so the passage
is typeset twice at the book's size: a wide column for the whole screen and a
narrow one beside a figure, which simply has more lines. Each is one tall
image with no page breaks, and the render scrolls it as the reader reads.
*/
type Typeset struct {
	Video   [2]int             `json:"video"`   // frame size the columns were set for
	Reading float64            `json:"lectura"` // where the line being read sits, as a share of the height
	Figure  TypesetFigure      `json:"figura"`
	Columns map[string]*Column `json:"columnas"`
}

// TypesetFigure is where a figure fixed on the right goes.
type TypesetFigure struct {
	X0     int `json:"x0"`     // its box starts here
	Margin int `json:"margen"` // and keeps this far from the frame's edges
}

// Column is one of the two typeset columns.
type Column struct {
	File   string `json:"archivo"` // relative to the reading's folder
	X      int    `json:"x"`       // where it sits in the frame
	Width  int    `json:"ancho"`
	Height int    `json:"alto"`
	// Paragraphs maps the number of a paragraph's first line (marcas.json)
	// to its lines: [offset in the text the studio reads of it, top, height].
	Paragraphs map[string][][3]int `json:"parrafos"`
}

// Column names.
const (
	Wide   = "ancha"
	Narrow = "angosta"
)

// LoadTypeset reads tipografia.json; nil, nil when the reading has none
// (lectura 07, or one prepared before it existed).
func LoadTypeset(dir string) (*Typeset, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "tipografia.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var t Typeset
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("tipografia.json: %w", err)
	}
	if t.Columns[Wide] == nil || t.Columns[Narrow] == nil {
		return nil, fmt.Errorf("tipografia.json has no %s or %s column", Wide, Narrow)
	}
	if t.Reading <= 0 || t.Reading >= 1 {
		t.Reading = 0.38
	}
	return &t, nil
}

// TypesetLine is one line of a column.
type TypesetLine struct {
	Offset int // where it starts in the paragraph's spoken text
	Top    int
	Height int
}

// Lines of paragraph p in this column, top to bottom.
func (c *Column) Lines(r *Reading, p Paragraph) []TypesetLine {
	raw := c.Paragraphs[strconv.Itoa(r.Lines[p.From].Number)]
	out := make([]TypesetLine, 0, len(raw))
	for _, l := range raw {
		out = append(out, TypesetLine{Offset: l[0], Top: l[1], Height: l[2]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Top < out[j].Top })
	return out
}

// LineAt is the line holding offset of the paragraph's spoken text.
func LineAt(lines []TypesetLine, offset int) (TypesetLine, bool) {
	if len(lines) == 0 {
		return TypesetLine{}, false
	}
	at := lines[0]
	for _, l := range lines {
		if l.Offset > offset {
			break
		}
		at = l
	}
	return at, true
}

// LineOffset is where passage line n starts in paragraph p's spoken text,
// or -1 when n is not one of its lines.
func (r *Reading) LineOffset(p Paragraph, n int) int {
	for _, m := range r.spokenText(p).lines {
		if m.line == n {
			return m.at
		}
	}
	return -1
}

// OutTime is where instant t of a take lands once cuts are taken out.
func OutTime(cuts []Cut, end, t float64) float64 {
	out := 0.0
	for _, s := range KeptSpans(cuts, end) {
		switch {
		case t >= s.To:
			out += s.Length()
		case t > s.From:
			out += t - s.From
		}
	}
	return out
}
