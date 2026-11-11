package api

import (
	"fmt"
	"image"
	"os"
	"path/filepath"

	"studio/internal/project"
	"studio/internal/render"
)

// arrowFor is how long the return arrow stays after a figure (2.d): long
// enough to find the line, short enough not to sit there for a paragraph.
const arrowFor = 4.0

// scrollPieces lays out a take that reads a paragraph, on the typeset column
// rather than the book's pages.
//
// The beats are the same as for the pages (project.Beats): the text, the
// figure with its caption while the caption is read (2.d), the figure held
// after it, the figure without caption for a later mention (2.e). What
// changes is the text: instead of a still of the page, the column scrolls so
// that the line being read stays at the same height, timed by where Whisper
// heard each line start. Beside a figure fixed on the right (2.f) the text is
// the narrow column, set at the same size with more lines; the column only
// changes at a cut (a figure or a new take), never under the viewer's eyes.
func (s *Server) scrollPieces(t project.Take, workDir string, paragraphs []project.Paragraph,
	afterFigure bool, figure func(string, bool) string, separate map[string]bool,
	hold float64) ([]takePiece, error) {

	ts := s.typeset
	width, height := ts.Video[0], ts.Video[1]
	if t.Paragraph < 0 || t.Paragraph >= len(paragraphs) {
		bg, err := s.background(workDir, "", width, height)
		if err != nil {
			return nil, err
		}
		col := ts.Columns[project.Wide]
		return []takePiece{{frame: bg, cuts: t.Cuts, scroll: &render.Scroll{
			Background: bg, Column: filepath.Join(s.cfg.ReadingDir, col.File), X: col.X, Window: height,
			Path: []render.ScrollPoint{{T: 0, Y: 0}},
		}}}, nil
	}
	p := paragraphs[t.Paragraph]
	beats, said, toSaid := s.reading.Beats(p, separate)

	// Seconds into the recording for a fraction of what was said: where
	// Whisper heard it, or spread by the text's length and moved to the
	// nearest pause. Same as for the pages.
	heard := func(float64) (float64, bool) { return 0, false }
	if a, ok := s.alignment(t); ok && len(said) > 0 {
		timer := a.Timer(said)
		heard = func(f float64) (float64, bool) { return timer(int(f*float64(len(said)) + 0.5)) }
	}
	at := func(f float64) float64 {
		if v, ok := heard(f); ok {
			return min(max(v, 0), t.Duration)
		}
		return t.TimeAt(f)
	}
	seconds := map[float64]float64{0: 0, 1: t.Duration}
	last := 0.0
	timeOf := func(f float64) float64 {
		v, ok := seconds[f]
		if !ok {
			v = at(f)
		}
		v = min(max(v, last), t.Duration)
		seconds[f], last = v, v
		return v
	}
	fraction := func(offset int) float64 {
		if len(said) == 0 {
			return 0
		}
		return min(1, float64(toSaid(offset))/float64(len(said)))
	}

	// A figure on the right stays for the whole run of text it shares, so
	// the column does not change in the middle of a sentence: a run that gets
	// its figure part of the way through has it from the start (2.f asks for
	// it from the start of the paragraph anyway).
	for i := 0; i < len(beats); {
		if beats[i].Kind != project.BeatPage {
			i++
			continue
		}
		j := i
		for j < len(beats) && beats[j].Kind == project.BeatPage {
			j++
		}
		fig := ""
		for k := i; k < j; k++ {
			if beats[k].Figure != "" {
				fig = beats[k].Figure
			} else {
				beats[k].Figure = fig
			}
		}
		fig = ""
		for k := j - 1; k >= i; k-- {
			if beats[k].Figure != "" {
				fig = beats[k].Figure
			} else {
				beats[k].Figure = fig
			}
		}
		i = j
	}

	var pieces []takePiece
	for k, b := range beats {
		switch b.Kind {
		case project.BeatHold:
			pieces = append(pieces, takePiece{frame: figure(b.Figure, b.WithCaption), fit: true,
				silent: true, seconds: hold})
			continue
		case project.BeatCaption:
			from, to := timeOf(b.From), timeOf(b.To)
			pieces = append(pieces, takePiece{frame: figure(b.Figure, true), fit: true,
				cuts: only(t, from, to)})
			continue
		}

		from, to := timeOf(b.From), timeOf(b.To)
		name := project.Wide
		if b.Figure != "" {
			name = project.Narrow
		}
		col := ts.Columns[name]
		lines := col.Lines(s.reading, p)

		// Where the line being read is, at instants of the recording.
		type key struct{ t, y float64 }
		var keys []key
		prev := 0.0
		for _, l := range lines {
			v := max(at(fraction(l.Offset)), prev)
			keys = append(keys, key{v, float64(l.Top)})
			prev = v
		}
		if n := len(lines); n > 0 {
			step := float64(lines[n-1].Height) * 1.2
			if n > 1 {
				step = float64(lines[n-1].Top - lines[n-2].Top)
			}
			keys = append(keys, key{max(at(1), prev), float64(lines[n-1].Top) + step})
		}
		reading := ts.Reading * float64(height)
		maxTop := float64(max(0, col.Height-height))
		top := func(v float64) float64 {
			y := 0.0
			switch {
			case len(keys) == 0:
			case v <= keys[0].t:
				y = keys[0].y
			case v >= keys[len(keys)-1].t:
				y = keys[len(keys)-1].y
			default:
				for i := 1; i < len(keys); i++ {
					if v <= keys[i].t {
						a, b := keys[i-1], keys[i]
						if b.t-a.t < 1e-6 {
							y = b.y
						} else {
							y = a.y + (v-a.t)*(b.y-a.y)/(b.t-a.t)
						}
						break
					}
				}
			}
			return min(max(y-reading, 0), maxTop)
		}

		cuts := only(t, from, to)
		path := []render.ScrollPoint{{T: 0, Y: top(from)}}
		for _, kf := range keys {
			if kf.t > from && kf.t < to {
				path = append(path, render.ScrollPoint{T: project.OutTime(cuts, t.Duration, kf.t), Y: top(kf.t)})
			}
		}
		path = append(path, render.ScrollPoint{T: project.OutTime(cuts, t.Duration, to), Y: top(to)})

		bg, err := s.background(workDir, b.Figure, width, height)
		if err != nil {
			return nil, err
		}
		scroll := &render.Scroll{
			Background: bg, Column: filepath.Join(s.cfg.ReadingDir, col.File),
			X: col.X, Window: height, Path: path,
		}
		// The return arrow, on the line the reading picks up at.
		resume := -1
		switch {
		case b.Resume > 0:
			resume = s.reading.LineOffset(p, b.Resume)
		case k == 0 && afterFigure:
			resume = 0
		}
		if resume >= 0 {
			if l, ok := project.LineAt(lines, resume); ok {
				arrow, err := s.arrowImage(workDir)
				if err != nil {
					return nil, err
				}
				scroll.Arrow, scroll.ArrowX = arrow, max(4, col.X-56)
				scroll.ArrowY, scroll.ArrowFor = l.Top+l.Height/2, arrowFor
			}
		}
		pieces = append(pieces, takePiece{frame: bg, cuts: cuts, scroll: scroll})
	}
	return pieces, nil
}

// background is the frame under the text: paper, with the figure on the right
// when one is fixed there. One file per figure per render.
func (s *Server) background(workDir, fig string, width, height int) (string, error) {
	name := "fondo.png"
	if fig != "" {
		name = fmt.Sprintf("fondo-%s.png", fig)
	}
	path := filepath.Join(workDir, name)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	file := ""
	if fig != "" {
		file = filepath.Join(s.cfg.ReadingDir, "figuras", fig+".png")
	}
	f := s.typeset.Figure
	box := image.Rect(f.X0, f.Margin, width-f.Margin, height-f.Margin)
	return path, render.DrawBackground(path, width, height, file, box)
}

func (s *Server) arrowImage(workDir string) (string, error) {
	path := filepath.Join(workDir, "flecha.png")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	return path, render.DrawArrowImage(path)
}
