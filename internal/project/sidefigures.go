package project

import (
	"slices"
	"sort"
	"strings"
	"unicode"
)

// spoken is a paragraph as the reader says it: no caption lines, one space
// between lines. Offsets are in runes, which follow speaking time more
// closely than bytes do.
type spoken struct {
	text  []rune
	lines []lineMark
}

type lineMark struct{ line, at int }

func (r *Reading) spokenText(p Paragraph) spoken {
	var s spoken
	for i := p.From; i <= p.To; i++ {
		l := r.Lines[i]
		t := strings.TrimSpace(l.Text)
		if l.Kind == "pie" || t == "" {
			continue
		}
		if len(s.text) > 0 {
			s.text = append(s.text, ' ')
		}
		s.lines = append(s.lines, lineMark{l.Number, len(s.text)})
		s.text = append(s.text, []rune(t)...)
	}
	return s
}

// startOf is where line n starts, or the next line after it that is read.
func (s spoken) startOf(n int) int {
	for _, m := range s.lines {
		if m.line >= n {
			return m.at
		}
	}
	return len(s.text)
}

// lineAt is the line holding offset at.
func (s spoken) lineAt(at int) int {
	line := 0
	for _, m := range s.lines {
		if m.at > at {
			break
		}
		line = m.line
	}
	return line
}

// SidePart is one figure kept on the right for part of a paragraph (2.f).
type SidePart struct {
	Figure string
	// At is where in the paragraph's text the figure takes over, 0..1.
	At float64
}

type sideEntry struct {
	figure string
	at     int
}

// SideFigures lists the figures fixed beside paragraph p, in the order the
// reading reaches them. Usually there is one, or none. When a paragraph talks
// about two («…la unidad de la Figura 27 (a la derecha)… La bonita molécula
// C60… mostrada en la Figura 30»), the first one is shown from the start and
// each later one takes over at the sentence that names it.
func (r *Reading) SideFigures(p Paragraph) []SidePart {
	s := r.spokenText(p)
	var out []SidePart
	for _, e := range r.sideEntries(p, s) {
		out = append(out, SidePart{Figure: e.figure, At: float64(e.at) / float64(len(s.text))})
	}
	return out
}

func (r *Reading) sideEntries(p Paragraph, s spoken) []sideEntry {
	if len(r.Fixed) == 0 || len(s.text) == 0 {
		return nil
	}
	first, last := r.Lines[p.From].Number, r.Lines[p.To].Number

	var found []sideEntry
	seen := map[string]bool{}
	for _, f := range r.Fixed {
		if f.To < first || f.From > last || seen[f.Figure] {
			continue
		}
		seen[f.Figure] = true
		at := 0
		switch {
		case f.From > first:
			// Fixed by hand from a line inside the paragraph.
			at = s.startOf(f.From)
		case f.From == first:
			// The stretch opens here, so the paragraph is why it is fixed:
			// it takes over where the text names it, if it does.
			if i := mention(s.text, r.FigureName(f.Figure), 0); i >= 0 {
				at = sentenceStart(s.text, i)
			}
		}
		// Carried over from an earlier paragraph: it is already on screen.
		found = append(found, sideEntry{f.Figure, at})
	}
	if len(found) == 0 {
		return nil
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].at < found[j].at })
	found[0].at = 0
	return found
}

// mention finds the figure's name in the text from offset from, ignoring
// case, and not as the start of a longer number («Figura 3» is not in
// «Figura 30»).
func mention(text []rune, name string, from int) int {
	needle := []rune(strings.ToLower(name))
	if len(needle) == 0 {
		return -1
	}
	lower := []rune(strings.ToLower(string(text)))
	for i := max(from, 0); i+len(needle) <= len(lower); i++ {
		if !slices.Equal(lower[i:i+len(needle)], needle) {
			continue
		}
		end := i + len(needle)
		if end < len(lower) && (unicode.IsDigit(lower[end]) || unicode.IsLetter(lower[end])) {
			continue
		}
		return i
	}
	return -1
}

const closers = "»”\")]"

// sentenceStart walks back from i to the start of its sentence.
func sentenceStart(text []rune, i int) int {
	for j := i - 1; j > 0; j-- {
		if text[j] != ' ' {
			continue
		}
		k := j - 1
		for k >= 0 && strings.ContainsRune(closers, text[k]) {
			k--
		}
		if k >= 0 && strings.ContainsRune(".!?…", text[k]) {
			return j + 1
		}
	}
	return 0
}

// sentenceEnd walks forward from i to just after the punctuation that closes
// its sentence, closing quotes and brackets included.
func sentenceEnd(text []rune, i int) int {
	for j := i; j < len(text); j++ {
		if !strings.ContainsRune(".!?…", text[j]) {
			continue
		}
		k := j + 1
		for k < len(text) && strings.ContainsRune(closers, text[k]) {
			k++
		}
		if k == len(text) || text[k] == ' ' {
			return k
		}
	}
	return len(text)
}

// afterMention is just past the figure's name and the punctuation that
// follows it: «(Figura 28).», «Figura 30,». That is where a reader stops to
// read the caption.
func afterMention(text []rune, i int, name string) int {
	j := i + len([]rune(name))
	for j < len(text) && strings.ContainsRune(".,;:"+closers, text[j]) {
		j++
	}
	return j
}
