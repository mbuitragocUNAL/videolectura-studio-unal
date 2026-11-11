package render

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

// paper is the page white the strip is printed on, used for the gaps between
// pages so the reading looks like one continuous sheet rather than a stack.
var paper = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}

// Strip is the page images laid end to end, as tira.py arranged them.
type Strip struct {
	Width int         `json:"ancho"`
	Gap   int         `json:"gap"`
	Pages []StripPage `json:"paginas"`
	dir   string
}

// StripPage is one page of the book as tira.py cropped it. Its own image is
// exactly Height tall; where it sits down the strip is the sum of the heights
// before it, which is how marcas.py places every line — `y` is the ink offset
// inside the original page and means nothing here.
type StripPage struct {
	File   string `json:"archivo"`
	Height int    `json:"alto"`
}

// offsets is where each page begins down the strip.
func (s Strip) offsets() []int {
	out := make([]int, len(s.Pages))
	running := 0
	for i, p := range s.Pages {
		out[i] = running
		running += p.Height
	}
	return out
}

// Height is the whole strip, which is what every position is a fraction of.
func (s Strip) Height() int {
	var total int
	for _, p := range s.Pages {
		total += p.Height
	}
	return total
}

// LoadStrip reads tira.json and the directory holding the page images.
func LoadStrip(readingDir string) (*Strip, error) {
	raw, err := os.ReadFile(filepath.Join(readingDir, "tira.json"))
	if err != nil {
		return nil, fmt.Errorf("reading tira.json: %w", err)
	}
	var strip Strip
	if err := json.Unmarshal(raw, &strip); err != nil {
		return nil, fmt.Errorf("parsing tira.json: %w", err)
	}
	if strip.Width <= 0 || len(strip.Pages) == 0 {
		return nil, fmt.Errorf("tira.json describes no pages")
	}
	strip.dir = filepath.Join(readingDir, "paginas")
	return &strip, nil
}

/*
Frame draws the stretch of the strip a take shows, and writes it as a PNG.

The video used to be the screen recording itself, which stopped working the
moment the page was made to hold still: a browser only emits a frame when the
tab changes, so a take of a motionless page came out at one frame per second.
Twenty-one minutes of audio arrived with fourteen minutes of pictures, and the
two drifted apart from the first splice.

Drawing the page here instead makes the picture exact by construction — it is
the same page image the reader saw, cropped to the same place — and lets the
delivery be a true full-frame 1920x1080 with the page edge to edge, rather than
whatever shape the window happened to be.
*/
type FrameOpts struct {
	Top    int
	Height int
	// Limit is the strip position the passage ends at. Nothing below it is
	// drawn: the page carries on into the next section, and the reading does
	// not. Zero draws the page whole.
	Limit int
	// Arrow marks the strip position the reading resumes at, for the frame
	// that follows the lámina. Zero draws none.
	Arrow int
	// Width is the frame's width. Zero is the strip's own width; wider puts
	// white either side of the page, which is how a reading with a fixed
	// figure keeps its text the same size on every frame.
	Width int
	// Side is a figure to keep on the right (lineamiento 2.f). The page is
	// then cut down to its text column, TextFrom..TextTo in strip pixels, and
	// set at the left at the same scale; the figure fills what is left.
	Side             string
	TextFrom, TextTo int
	// Hide are stretches of the strip painted over with paper: the figure
	// fixed on the right where it is also printed in the page, so it is not
	// on screen twice.
	Hide []image.Rectangle
}

func (s *Strip) Frame(top, height int, path string) error {
	return s.Render(FrameOpts{Top: top, Height: height}, path)
}

func (s *Strip) Render(o FrameOpts, path string) error {
	height := o.Height
	if height <= 0 {
		return fmt.Errorf("a frame of %d pixels is not a frame", height)
	}
	top := min(max(o.Top, 0), max(s.Height()-height, 0))

	width := max(o.Width, s.Width)
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	// The gaps between pages are page-coloured, not black: the strip is meant
	// to read as one continuous sheet.
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(paper), image.Point{}, draw.Src)

	// Where the strip's x = 0 lands on the canvas, and how much of the canvas
	// the page may use.
	shift, pageRight := (width-s.Width)/2, width
	margin := width / 40
	if o.Side != "" {
		shift = margin - o.TextFrom
		pageRight = margin + (o.TextTo - o.TextFrom)
	}

	at := s.offsets()
	for i, page := range s.Pages {
		// Only the pages the window actually crosses.
		if at[i]+page.Height <= top || at[i] >= top+height {
			continue
		}
		img, err := s.open(page.File)
		if err != nil {
			return err
		}
		origin := image.Pt(shift, at[i]-top)
		dst := image.Rectangle{Min: origin, Max: origin.Add(img.Bounds().Size())}
		dst = dst.Intersect(image.Rect(0, 0, pageRight, height))
		draw.Draw(canvas, dst, img, img.Bounds().Min.Add(dst.Min.Sub(origin)), draw.Src)
	}

	for _, h := range o.Hide {
		r := h.Add(image.Pt(shift, -top)).Intersect(image.Rect(0, 0, pageRight, height))
		if !r.Empty() {
			draw.Draw(canvas, r, image.NewUniform(paper), image.Point{}, draw.Src)
		}
	}

	// The passage stops before the page does. Everything past its last line
	// belongs to the next section, which this reading does not cover, and
	// leaving it on screen puts text in front of the viewer that nobody is
	// going to read.
	if o.Limit > 0 && o.Limit < top+height {
		blank := image.Rect(0, max(o.Limit-top, 0), pageRight, height)
		draw.Draw(canvas, blank, image.NewUniform(paper), image.Point{}, draw.Src)
	}

	if o.Side != "" {
		figure, err := decodePNG(o.Side)
		if err != nil {
			return err
		}
		box := image.Rect(pageRight+margin, margin, width-margin, height-margin)
		drawFitted(canvas, figure, box)
	}

	if o.Arrow > 0 {
		left := shift + 54
		if o.Side != "" {
			left = max(4, margin-50)
		}
		drawArrow(canvas, o.Arrow-top, left)
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(out, canvas); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// arrowInk is the red of the marker the reader sees on returning from the
// lámina. Lineamiento 3.2.3 asks for it, so it is not an overlay of ours: it
// is part of what the page is supposed to show at that moment.
var arrowInk = color.RGBA{R: 0xc4, G: 0x16, B: 0x1c, A: 0xff}

// drawArrow puts a solid triangle in the left margin, pointing at the line the
// reading picks up from.
func drawArrow(canvas *image.RGBA, y, left int) {
	const size = 46 // height of the arrow, in strip pixels
	if y < 0 || y > canvas.Bounds().Dy() {
		return
	}
	for row := -size / 2; row <= size/2; row++ {
		// A triangle: the further from the middle, the shorter the row.
		width := size - 2*abs(row)
		if width <= 0 {
			continue
		}
		for x := left; x < left+width; x++ {
			at := image.Pt(x, y+row)
			if at.In(canvas.Bounds()) {
				canvas.Set(at.X, at.Y, arrowInk)
			}
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (s *Strip) open(name string) (image.Image, error) {
	return decodePNG(filepath.Join(s.dir, name))
}

func decodePNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", filepath.Base(path), err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", filepath.Base(path), err)
	}
	return img, nil
}

// drawFitted scales img to fit inside box, keeping its proportions, and
// centres it there. Each canvas pixel averages the block of source pixels it
// covers, which is what a reduction needs to stay sharp without aliasing; an
// enlargement degrades to nearest neighbour, which at these sizes is rare.
func drawFitted(canvas *image.RGBA, img image.Image, box image.Rectangle) {
	src := img.Bounds()
	if src.Empty() || box.Empty() {
		return
	}
	scale := min(float64(box.Dx())/float64(src.Dx()), float64(box.Dy())/float64(src.Dy()))
	w, h := int(float64(src.Dx())*scale), int(float64(src.Dy())*scale)
	x0 := box.Min.X + (box.Dx()-w)/2
	y0 := box.Min.Y + (box.Dy()-h)/2
	for y := 0; y < h; y++ {
		sy0 := src.Min.Y + int(float64(y)/scale)
		sy1 := max(sy0+1, src.Min.Y+int(float64(y+1)/scale))
		for x := 0; x < w; x++ {
			sx0 := src.Min.X + int(float64(x)/scale)
			sx1 := max(sx0+1, src.Min.X+int(float64(x+1)/scale))
			var r, g, b, n uint32
			for sy := sy0; sy < sy1 && sy < src.Max.Y; sy++ {
				for sx := sx0; sx < sx1 && sx < src.Max.X; sx++ {
					cr, cg, cb, _ := img.At(sx, sy).RGBA()
					r, g, b, n = r+cr, g+cg, b+cb, n+1
				}
			}
			if n == 0 {
				continue
			}
			canvas.SetRGBA(x0+x, y0+y, color.RGBA{
				R: uint8(r / n >> 8), G: uint8(g / n >> 8), B: uint8(b / n >> 8), A: 0xff,
			})
		}
	}
}
