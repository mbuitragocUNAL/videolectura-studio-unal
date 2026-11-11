package render

import (
	"image"
	"image/color"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The expression is linear between points and nests shallowly.
func TestPathExprIsABalancedTree(t *testing.T) {
	var path []ScrollPoint
	for i := 0; i <= 64; i++ {
		path = append(path, ScrollPoint{T: float64(i), Y: float64(10 * i)})
	}
	expr := pathExpr(path)
	depth, deepest := 0, 0
	for _, c := range expr {
		switch c {
		case '(':
			depth++
			deepest = max(deepest, depth)
		case ')':
			depth--
		}
	}
	if deepest > 20 {
		t.Errorf("64 segments nest %d deep", deepest)
	}
	if !strings.HasPrefix(expr, "if(lt(t,32.000)") {
		t.Errorf("the root does not split in the middle: %.40s", expr)
	}
}

// A real piece: the column scrolls 1000 px over 4 s, so at 2 s the window's
// top is at 500, and the arrow, drawn on the column, scrolls with it.
func TestScrollPieceMovesTheColumn(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	dir := t.TempDir()
	// A column whose grey level is its own height: 0 at the top, 250 at 3000.
	col := image.NewRGBA(image.Rect(0, 0, 200, 3000))
	for y := 0; y < 3000; y++ {
		g := uint8(y / 12)
		for x := 0; x < 200; x++ {
			col.Set(x, y, color.RGBA{g, g, g, 255})
		}
	}
	colPath := filepath.Join(dir, "col.png")
	if err := writePNGFile(colPath, col); err != nil {
		t.Fatal(err)
	}
	bg := filepath.Join(dir, "bg.png")
	if err := DrawBackground(bg, 640, 360, "", image.Rectangle{}); err != nil {
		t.Fatal(err)
	}
	arrow := filepath.Join(dir, "arrow.png")
	if err := DrawArrowImage(arrow); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		if out, err := exec.Command("ffmpeg", append([]string{"-v", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	voice := filepath.Join(dir, "voice.wav")
	run("-f", "lavfi", "-i", "sine=frequency=200:duration=4", voice)

	plan := TakePlan{Source: voice, Frame: bg, Duration: 4, Height: 360, Scroll: &Scroll{
		Background: bg, Column: colPath, X: 100, Window: 360,
		Path:  []ScrollPoint{{0, 0}, {4, 1000}},
		Arrow: arrow, ArrowX: 40, ArrowY: 800, ArrowFor: 10,
	}}
	out := filepath.Join(dir, "piece.mp4")
	cmd := BuildArgs(plan).CommandLine(out)
	if b, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	frame := filepath.Join(dir, "at2.png")
	run("-ss", "2", "-i", out, "-frames:v", "1", frame)
	img, err := decodePNG(frame)
	if err != nil {
		t.Fatal(err)
	}
	// Top of the window: column y 500, grey 41.
	r, _, _, _ := img.At(200, 1).RGBA()
	if g := int(r >> 8); g < 36 || g > 47 {
		t.Errorf("at 2 s the top of the window is grey %d, want about 41 (column y 500)", g)
	}
	// The arrow at column y 800 is 300 px below the window's top at 2 s.
	red := 0
	for y := 280; y < 320; y++ {
		r, g, _, _ := img.At(50, y).RGBA()
		if r>>8 > 150 && g>>8 < 80 {
			red++
		}
	}
	if red == 0 {
		t.Error("no arrow where the scrolled text puts it")
	}
}
