package project

import "sort"

// Beat kinds: what the screen shows during one stretch of a paragraph's take.
const (
	// BeatPage is the page, with Figure fixed on the right if it is set (2.f).
	BeatPage = "pagina"
	// BeatCaption is the figure with its caption, full screen, while the
	// reader reads the caption (2.d).
	BeatCaption = "pie"
	// BeatHold is the figure held over silence: with its caption after it has
	// been read (2.d), without it for a later mention (2.e). It takes no time
	// from the recording.
	BeatHold = "pausa"
)

// Beat is one stretch of a paragraph's take, placed in its text. From and To
// are fractions of what the reader says, caption included, so a take's
// loudness can turn them into seconds (Take.TimeAt).
type Beat struct {
	Kind   string
	Figure string
	From   float64
	To     float64
	// WithCaption is for a hold: whether the figure keeps its caption.
	WithCaption bool
	// Resume is the line the reading picks up at after a figure, where the
	// return arrow goes. Zero when the page does not follow a figure.
	Resume int
}

// Beats lays out what the screen shows while paragraph p is read: the page,
// and the figures its sentences send the viewer to, in the order the reader
// meets them. separate says which figures had their caption recorded as a
// take of its own; any other first mention is taken to have been read where
// the text names it, which is how the lineamientos ask for it (2.d: carry on
// to the end of the sentence, show the figure, read its caption).
//
// It also returns the text as the reader said it, captions in place, which is
// what the fractions in the beats are fractions of, and toSaid, which takes a
// place in the printed paragraph (its spoken text, without captions) to the
// same place in what was said.
func (r *Reading) Beats(p Paragraph, separate map[string]bool) (beats []Beat, said []rune, toSaid func(int) int) {
	s := r.spokenText(p)
	if len(s.text) == 0 {
		return []Beat{{Kind: BeatPage, From: 0, To: 1}}, nil, func(at int) int { return at }
	}
	first, last := r.Lines[p.From].Number, r.Lines[p.To].Number

	// Everything that happens inside the paragraph, at an offset in its text.
	type moment struct {
		at      int
		kind    string // "side", "caption", "brief"
		figure  string
		caption []rune // read aloud here, for "caption"
		resume  int    // line the reading resumes at
	}
	var moments []moment
	introduced := map[string]bool{} // first mentioned in this paragraph
	for _, e := range r.Events {
		if e.Line < first || e.Line > last {
			continue
		}
		if e.Mode == ModeCaption {
			introduced[e.Figure] = true
		}
		name := r.FigureName(e.Figure)
		i := mention(s.text, name, s.startOf(e.Line))
		if i < 0 {
			i = mention(s.text, name, 0)
		}
		switch {
		case i < 0:
			// Named in a way the text search cannot see: leave it for after
			// the paragraph rather than guess where.
			continue
		case e.Mode == ModeCaption && !separate[e.Figure]:
			at := afterMention(s.text, i, name)
			moments = append(moments, moment{at: at, kind: "caption", figure: e.Figure,
				caption: []rune(r.caption(e.Figure)), resume: s.lineAt(at)})
		case e.Mode == ModeBrief:
			at := sentenceEnd(s.text, i)
			moments = append(moments, moment{at: at, kind: "brief", figure: e.Figure,
				resume: s.lineAt(min(at+1, len(s.text)-1))})
		}
	}

	// Figures fixed on the right. One whose caption is read in this
	// paragraph waits for it: a figure is first seen whole, with its caption
	// (2.d), and only then stays beside the text. One whose caption was
	// recorded apart waits for the next paragraph, after that take.
	side := ""
	for _, e := range r.sideEntries(p, s) {
		// Its caption is a take of its own, after this paragraph.
		if introduced[e.figure] && separate[e.figure] {
			continue
		}
		for _, m := range moments {
			if m.kind == "caption" && m.figure == e.figure && m.at >= e.at {
				e.at = m.at
			}
		}
		if e.at == 0 && side == "" {
			side = e.figure
			continue
		}
		moments = append(moments, moment{at: e.at, kind: "side", figure: e.figure})
	}
	// In text order; at the same place the caption goes before the figure it
	// leaves fixed on the right.
	sort.SliceStable(moments, func(i, j int) bool {
		if moments[i].at != moments[j].at {
			return moments[i].at < moments[j].at
		}
		return moments[i].kind != "side" && moments[j].kind == "side"
	})

	// The text as spoken: each caption read where the reader stopped for it.
	total := len(s.text)
	said = make([]rune, 0, total)
	next := 0
	for _, m := range moments {
		if m.kind == "caption" {
			said = append(said, s.text[next:m.at]...)
			said = append(said, ' ')
			said = append(said, m.caption...)
			next = m.at
		}
	}
	said = append(said, s.text[next:]...)
	for _, m := range moments {
		if m.kind == "caption" {
			total += len(m.caption) + 1
		}
	}
	frac := func(at int) float64 { return float64(at) / float64(total) }

	var out []Beat
	page := func(from, to, resume int) {
		if to > from {
			out = append(out, Beat{Kind: BeatPage, Figure: side, From: frac(from), To: frac(to), Resume: resume})
		}
	}
	cursor, shift, resume := 0, 0, 0
	for _, m := range moments {
		at := m.at + shift
		switch m.kind {
		case "side":
			if at > cursor {
				page(cursor, at, resume)
				cursor, resume = at, 0
			}
			side = m.figure
		case "caption":
			page(cursor, at, resume)
			end := at + 1 + len(m.caption)
			out = append(out,
				Beat{Kind: BeatCaption, Figure: m.figure, From: frac(at), To: frac(end)},
				Beat{Kind: BeatHold, Figure: m.figure, From: frac(end), To: frac(end), WithCaption: true})
			shift += len(m.caption) + 1
			cursor, resume = end, m.resume
		case "brief":
			page(cursor, at, resume)
			out = append(out, Beat{Kind: BeatHold, Figure: m.figure, From: frac(at), To: frac(at)})
			cursor, resume = at, m.resume
		}
	}
	page(cursor, total, resume)
	if len(out) == 0 {
		out = []Beat{{Kind: BeatPage, Figure: side, From: 0, To: 1}}
	}
	type insert struct{ at, length int }
	var inserts []insert
	for _, m := range moments {
		if m.kind == "caption" {
			inserts = append(inserts, insert{m.at, len(m.caption) + 1})
		}
	}
	toSaid = func(at int) int {
		out := at
		for _, in := range inserts {
			if in.at <= at {
				out += in.length
			}
		}
		return out
	}
	return out, said, toSaid
}

// HandledInline says whether Beats places this event inside the paragraph's
// take, so nothing more needs to be shown for it after the paragraph.
func (r *Reading) HandledInline(p Paragraph, e FigureEvent, separate map[string]bool) bool {
	if e.Mode == ModeCaption && separate[e.Figure] {
		return false
	}
	s := r.spokenText(p)
	return mention(s.text, r.FigureName(e.Figure), 0) >= 0
}

func (r *Reading) caption(id string) string {
	for _, f := range r.Figures {
		if f.ID == id {
			return f.Caption
		}
	}
	return ""
}
