package render

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writePNG(t *testing.T, path string, w, h int, c color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// Lineamiento 2.f: the text column keeps its scale at the left and the figure
// takes the right. Nothing of the page may show to the right of its column,
// and the figure must actually be there.
func TestRenderKeepsTheFigureOnTheRight(t *testing.T) {
	dir := t.TempDir()
	ink := color.RGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xff}
	red := color.RGBA{R: 0xff, A: 0xff}
	writePNG(t, filepath.Join(dir, "p.png"), 400, 300, ink)
	writePNG(t, filepath.Join(dir, "fig.png"), 100, 100, red)
	strip := &Strip{Width: 400, Pages: []StripPage{{File: "p.png", Height: 300}}, dir: dir}

	out := filepath.Join(dir, "frame.png")
	err := strip.Render(FrameOpts{
		Top: 0, Height: 300, Width: 533,
		Side: filepath.Join(dir, "fig.png"), TextFrom: 100, TextTo: 300,
	}, out)
	if err != nil {
		t.Fatal(err)
	}
	img, err := decodePNG(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Dx(); got != 533 {
		t.Fatalf("frame is %d wide, want 533", got)
	}
	margin := 533 / 40
	// Inside the text column: page ink.
	if r, _, _, _ := img.At(margin+50, 150).RGBA(); r>>8 != 0x20 {
		t.Errorf("text column is not the page: red %d", r>>8)
	}
	// Just right of the column: paper, not the rest of the page.
	if r, g, _, _ := img.At(margin+200+5, 20).RGBA(); r>>8 != 0xff || g>>8 != 0xff {
		t.Errorf("page leaks past its column")
	}
	// The middle of the figure's box: the figure.
	if r, g, _, _ := img.At((margin+200+533)/2+6, 150).RGBA(); r>>8 != 0xff || g>>8 != 0 {
		t.Errorf("no figure on the right")
	}
}

// Without a side figure a wider frame centres the page.
func TestRenderCentresThePageInAWiderFrame(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "p.png"), 400, 300, color.RGBA{A: 0xff})
	strip := &Strip{Width: 400, Pages: []StripPage{{File: "p.png", Height: 300}}, dir: dir}
	out := filepath.Join(dir, "frame.png")
	if err := strip.Render(FrameOpts{Height: 300, Width: 600}, out); err != nil {
		t.Fatal(err)
	}
	img, _ := decodePNG(out)
	if r, _, _, _ := img.At(50, 150).RGBA(); r>>8 != 0xff {
		t.Errorf("left margin should be paper")
	}
	if r, _, _, _ := img.At(300, 150).RGBA(); r>>8 != 0 {
		t.Errorf("the page should be centred")
	}
}

// A figure fixed on the right is painted over where the page prints it, so
// the viewer does not see it twice.
func TestRenderHidesTheFigureItKeepsOnTheSide(t *testing.T) {
	dir := t.TempDir()
	ink := color.RGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xff}
	writePNG(t, filepath.Join(dir, "p.png"), 400, 600, ink)
	writePNG(t, filepath.Join(dir, "fig.png"), 50, 50, color.RGBA{R: 0xff, A: 0xff})
	strip := &Strip{Width: 400, Pages: []StripPage{{File: "p.png", Height: 600}}, dir: dir}

	out := filepath.Join(dir, "frame.png")
	err := strip.Render(FrameOpts{Top: 100, Height: 300, Width: 800,
		Side: filepath.Join(dir, "fig.png"), TextFrom: 0, TextTo: 400,
		Hide: []image.Rectangle{image.Rect(0, 200, 400, 300)}}, out)
	if err != nil {
		t.Fatal(err)
	}
	img, err := decodePNG(out)
	if err != nil {
		t.Fatal(err)
	}
	margin := 800 / 40
	if r, _, _, _ := img.At(margin+200, 150).RGBA(); r>>8 == 0x20 {
		t.Error("strip y 250 is still the printed figure")
	}
	if r, _, _, _ := img.At(margin+200, 50).RGBA(); r>>8 != 0x20 {
		t.Error("strip y 150, outside the figure, was painted over")
	}
}
