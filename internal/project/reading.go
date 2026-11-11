// Package project holds the studio's domain model: the reading being recorded
// and the mutable session built on top of it.
package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Reading is the immutable description of the passage to record: where every
// quote sits in the scrolling strip, how the quotes group into exchanges, and
// the text itself for the teleprompter.
//
// It is produced by the preparation scripts, whose JSON keys are Spanish; the
// struct tags carry that wire format so the Go side can stay in English.
type Reading struct {
	Pages    int       `json:"paginas"`
	StripPx  int       `json:"altoTira"`
	Quotes   []Quote   `json:"clips"`
	Groups   []Group   `json:"grupos"`
	Sections []Section `json:"secciones"`
	Plate    *Plate    `json:"lamina"`
	Lines    []Line    `json:"lineas"`
	// PassageEnd is where the assigned text stops, in strip pixels. Written by
	// preparar/lector.py, which knows it exactly; zero for lectura 07, whose
	// marcas.json predates it.
	PassageEnd int `json:"finPasaje"`
	// About names the reading (number, title, video title). The server only
	// passes it through to the console, which shows it in the header.
	About json.RawMessage `json:"lectura,omitempty"`
	// Figures and Events come from preparar/lector.py. Lectura 07 has neither
	// and relies on Plate alone.
	Figures []Figure      `json:"figuras"`
	Events  []FigureEvent `json:"eventos"`
	Screen  Screen        `json:"pantalla"`
	// Fixed are the stretches where a figure stays on the right while the
	// paragraphs that talk about it are read (lineamiento 2.f).
	Fixed []FixedFigure `json:"persistencias"`
	// TextBlock is where the text column sits across the strip, in strip
	// pixels: what a frame keeps when the figure takes the right side.
	TextBlock TextBlock `json:"texto"`
	// Music is lectura.toml's [musica], when the reading has a track under it.
	Music *Music `json:"musica,omitempty"`
}

// Music is the background track a reading asks for (lineamiento 2.j). File is
// a name in the studio's music folder.
type Music struct {
	File  string  `json:"archivo"`
	Below float64 `json:"nivel"`   // dB under the narration
	Duck  bool    `json:"atenuar"` // lower it further while the reader speaks
}

// FixedFigure is a run of passage lines, 1-based and inclusive, that talks
// about one figure.
type FixedFigure struct {
	Figure string `json:"figura"`
	From   int    `json:"nDesde"`
	To     int    `json:"nHasta"`
}

// TextBlock is the horizontal extent of the printed text.
type TextBlock struct {
	X0 float64 `json:"x0"`
	X1 float64 `json:"x1"`
}

// FixedFigureOf is the figure to keep on the right while a paragraph is read,
// or "": the one whose stretch overlaps the paragraph most.
func (r *Reading) FixedFigureOf(p Paragraph) string {
	first, last := r.Lines[p.From].Number, r.Lines[p.To].Number
	best, most := "", 0
	for _, f := range r.Fixed {
		overlap := min(last, f.To) - max(first, f.From) + 1
		if overlap > most {
			best, most = f.Figure, overlap
		}
	}
	return best
}

// Figure is an illustration the author names («Figura 27», «Lámina FF»),
// cropped into figuras/<id>.png and figuras/<id>-pie.png, the second with its
// caption.
type Figure struct {
	ID   string `json:"id"`
	Name string `json:"nombre"`
	// Caption is the printed caption's text. A reader who reads it inside
	// the paragraph's take is placed by it, so the render can show the figure
	// with its caption for exactly that stretch of the recording.
	Caption string `json:"pie,omitempty"`
	// Box is where the figure and its caption are printed in the strip, in
	// strip pixels (x0, y0, x1, y1); empty for one printed outside it.
	Box []int `json:"caja,omitempty"`
}

// FigureEvent is one mention of a figure, placed at the end of the sentence
// that names it. Lineamiento 3.2.3 treats the first and the rest differently:
// the first shows the figure whole with its caption, which is read (2.d); every
// later one shows it again without the caption for two or three seconds (2.e).
// Either way, the page that follows carries a return arrow.
type FigureEvent struct {
	Figure string  `json:"figura"`
	Mode   string  `json:"modo"` // ModeCaption or ModeBrief
	Line   int     `json:"n"`
	Start  float64 `json:"f"`
}

const (
	ModeCaption = "pie"
	ModeBrief   = "breve"
)

// Screen holds the display settings of lectura.toml's [pantalla].
type Screen struct {
	// FigureHold is how long a figure stays up on its own, in seconds.
	FigureHold float64 `json:"segundosFigura"`
	// FigureWidth is the share of the frame the fixed figure takes (2.f).
	FigureWidth float64 `json:"anchoFigura"`
}

// FigureName returns the printed name of a figure, or its id if unknown.
func (r *Reading) FigureName(id string) string {
	for _, f := range r.Figures {
		if f.ID == id {
			return f.Name
		}
	}
	return id
}

// FigureBox is where a figure is printed in the strip, or nil.
func (r *Reading) FigureBox(id string) []int {
	for _, f := range r.Figures {
		if f.ID == id {
			return f.Box
		}
	}
	return nil
}

// ParagraphOfLine is the paragraph holding passage line n (1-based), or -1.
func (r *Reading) ParagraphOfLine(n int) int {
	for _, p := range r.Paragraphs() {
		if n >= r.Lines[p.From].Number && n <= r.Lines[p.To].Number {
			return p.Index
		}
	}
	return -1
}

// Quote is one synthesised passage: a character speaking, with the stretch of
// the strip it occupies and the audio that renders it.
type Quote struct {
	ID        string   `json:"clip"`
	Character string   `json:"personaje"`
	Line      int      `json:"linea"`
	LastLine  *int     `json:"lineaFin"`
	Start     float64  `json:"f"`    // position in the strip, 0..1
	End       *float64 `json:"fFin"` // nil for a quote that sits mid-sentence
	Duration  float64  `json:"dur"`  // seconds
	Text      string   `json:"texto"`
}

// Inline reports whether the quote has no block of its own, in which case the
// scroll must not be driven by it.
func (q Quote) Inline() bool { return q.End == nil }

// Group is a run of quotes that form a single exchange and are played back to
// back. Gaps holds the pause before each quote after the first, in seconds.
type Group struct {
	Quotes []string  `json:"clips"`
	Gaps   []float64 `json:"pausas"`
}

// Section is a heading in the passage, used to label the timeline.
type Section struct {
	Title string  `json:"titulo"`
	Line  int     `json:"linea"`
	Start float64 `json:"f"`
}

// Plate is an illustration that must be shown full screen while its caption is
// read.
type Plate struct {
	Line  int     `json:"linea"`
	Start float64 `json:"f"`
	// Figure names the crop in figuras/ (<figura>-pie.png). Empty for lectura
	// 07, whose plate is paginas/laminaF-446.png.
	Figure string `json:"figura"`
}

// Line is one line of the passage with its position in the strip.
type Line struct {
	Number int     `json:"n"`
	Start  float64 `json:"f"`
	Text   string  `json:"t"`
	// Kind is texto, titulo or pie, from lector.py. A printed caption is read
	// with its figure, so the console leaves it out of the paragraph's text.
	Kind string `json:"tipo,omitempty"`
}

// QuoteByID returns the quote with the given id.
func (r *Reading) QuoteByID(id string) (Quote, bool) {
	for _, q := range r.Quotes {
		if q.ID == id {
			return q, true
		}
	}
	return Quote{}, false
}

// GroupDuration is how long the whole exchange takes: every clip plus the
// pauses between turns.
func (r *Reading) GroupDuration(g Group) float64 {
	var total float64
	for _, id := range g.Quotes {
		if q, ok := r.QuoteByID(id); ok {
			total += q.Duration
		}
	}
	for _, gap := range g.Gaps {
		total += gap
	}
	return total
}

// LineAt returns the last line of the passage at or before a strip position,
// which is how a moment of a take becomes something a reader can point at.
func (r *Reading) LineAt(f float64) (Line, bool) {
	if len(r.Lines) == 0 {
		return Line{}, false
	}
	lo, hi := 0, len(r.Lines)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if r.Lines[mid].Start <= f {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return r.Lines[lo], true
}

// LoadReading reads the prepared description of a passage.
func LoadReading(dir string) (*Reading, error) {
	path := filepath.Join(dir, "marcas.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var r Reading
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	// A passage with no quotations is legitimate: lineamiento 2.i only applies
	// when the author quotes someone.
	if len(r.Lines) == 0 {
		return nil, fmt.Errorf("%s has no lines", path)
	}
	return &r, nil
}
