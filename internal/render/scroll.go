package render

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"strings"
)

/*
Scroll is a page that moves.

The pages used to be stills: one frame of the book per paragraph, cut to the
next when the take changed. With the passage typeset as one tall column
(project.Typeset) the picture can follow the reading instead: the column
slides up so the line being read stays at the same height, as fast as the
reader goes and no faster, and a paragraph runs into the next with no cut.
*/
type Scroll struct {
	// Background is the frame without the text: white, with the figure on
	// the right when one is fixed there (lineamiento 2.f).
	Background string
	Column     string // the tall image of typeset text
	X          int    // where the column's left edge sits in the frame
	Window     int    // how much of the column shows: the frame's height
	// Path is where the window's top is, in column pixels, at instants of
	// the piece (seconds from its start, after cuts). In between it moves at
	// constant speed; past the ends it keeps going the same way.
	Path []ScrollPoint
	// Arrow marks the line the reading resumes at after a figure (2.d). It
	// is drawn on the column, so it scrolls with the text, for ArrowFor
	// seconds from the start of the piece.
	Arrow    string
	ArrowX   int
	ArrowY   int // centre, in column pixels
	ArrowFor float64
}

// ScrollPoint is the window's top at an instant.
type ScrollPoint struct{ T, Y float64 }

// pathExpr is Path as an ffmpeg expression of t: linear between points, as a
// balanced tree of if()s so that a long take with a point per line nests a
// handful deep rather than a hundred.
func pathExpr(path []ScrollPoint) string {
	switch len(path) {
	case 0:
		return "0"
	case 1:
		return fmt.Sprintf("%.1f", path[0].Y)
	}
	segment := func(i int) string {
		a, b := path[i], path[i+1]
		if b.T-a.T < 1e-3 {
			return fmt.Sprintf("%.1f", b.Y)
		}
		return fmt.Sprintf("%.1f+(t-%.3f)*%.4f", a.Y, a.T, (b.Y-a.Y)/(b.T-a.T))
	}
	var node func(lo, hi int) string
	node = func(lo, hi int) string {
		if lo == hi {
			return segment(lo)
		}
		mid := (lo + hi) / 2
		return fmt.Sprintf("if(lt(t,%.3f),%s,%s)", path[mid+1].T, node(lo, mid), node(mid+1, hi))
	}
	return node(0, len(path)-2)
}

// scrollGraph is the video half of the graph for a scrolling piece: input 0
// is the background (looped by -loop), column and arrow are the inputs at
// the given indices, decoded once and repeated with the loop filter, because
// re-reading a column twenty thousand pixels tall for every frame would take
// longer than the reading.
func scrollGraph(s *Scroll, column, arrow, width, height int) string {
	y := pathExpr(s.Path)
	parts := []string{
		fmt.Sprintf("[%d:v]loop=loop=-1:size=1:start=0,fps=25,crop=w=iw:h=%d:x=0:y='%s'[col]", column, s.Window, y),
		fmt.Sprintf("[0:v][col]overlay=x=%d:y=0:shortest=0[pg]", s.X),
	}
	last := "[pg]"
	if s.Arrow != "" && arrow >= 0 {
		parts = append(parts,
			fmt.Sprintf("[%d:v]loop=loop=-1:size=1:start=0,fps=25[ar]", arrow),
			fmt.Sprintf("[pg][ar]overlay=x=%d:y='%d-(%s)-h/2':eval=frame:enable='lt(t,%.2f)'[pa]",
				s.ArrowX, s.ArrowY, y, s.ArrowFor))
		last = "[pa]"
	}
	parts = append(parts, fmt.Sprintf("%sscale=%d:%d,setsar=1[v]", last, width, height))
	return strings.Join(parts, ";")
}

// DrawBackground writes a frame of paper with a figure fitted into box, or
// plain paper when figure is empty.
func DrawBackground(path string, width, height int, figure string, box image.Rectangle) error {
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(paper), image.Point{}, draw.Src)
	if figure != "" {
		img, err := decodePNG(figure)
		if err != nil {
			return err
		}
		drawFitted(canvas, img, box)
	}
	return writePNGFile(path, canvas)
}

// DrawArrowImage writes the return arrow alone, on transparent ground.
func DrawArrowImage(path string) error {
	canvas := image.NewRGBA(image.Rect(0, 0, 50, 50))
	drawArrow(canvas, 25, 2)
	return writePNGFile(path, canvas)
}

func writePNGFile(path string, img image.Image) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(out, img); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
