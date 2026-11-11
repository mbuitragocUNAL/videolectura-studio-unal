package project

import (
	"math"
	"strings"
	"testing"
)

// The caption starts at the second «Figura 30», which Whisper hears even
// though it mangles «buckyball» and «C60». The switch goes in the pause
// before it, a breath ahead of the word.
func TestTimerPutsTheSwitchBeforeTheWord(t *testing.T) {
	said := []rune("La bonita molécula C60, mostrada en la Figura 30, FIGURA 30. Estructura de la buckyball. Es particularmente estable.")
	a := Alignment{Words: []Word{
		{"La", 49.9, 50.2}, {"bonita", 50.7, 51.1}, {"molécula", 51.1, 51.7}, {"660", 51.7, 52.3},
		{"mostrada", 53.6, 54.1}, {"en", 54.1, 54.2}, {"la", 54.2, 54.3}, {"figura", 54.3, 54.5}, {"30,", 54.5, 55.0},
		{"figura", 55.64, 55.9}, {"30,", 55.94, 56.4}, {"estructura", 56.7, 57.1}, {"de", 57.16, 57.3},
		{"la", 57.3, 57.4}, {"bakibol,", 57.4, 58.0},
		{"Es", 66.46, 66.8}, {"particularmente", 66.9, 67.7}, {"estable.", 67.8, 68.1},
	}}
	timer := a.Timer(said)
	for _, c := range []struct {
		before string
		want   float64
	}{
		{"La bonita molécula C60, mostrada en la Figura 30, ", 55.52},
		{"La bonita molécula C60, mostrada en la Figura 30, FIGURA 30. Estructura de la buckyball. ", 66.34},
	} {
		got, ok := timer(len([]rune(c.before)))
		if !ok || math.Abs(got-c.want) > 0.01 {
			t.Errorf("after %q got %.2f (%v), want %.2f", c.before, got, ok, c.want)
		}
	}
}

// A transcript that matches almost nothing is not trusted.
func TestTimerRefusesAPoorTranscript(t *testing.T) {
	a := Alignment{Words: []Word{{"nada", 0, 1}, {"que", 1, 2}, {"ver", 2, 3}}}
	if _, ok := a.Timer([]rune("La bonita molécula de carbono es estable."))(10); ok {
		t.Error("a transcript of other words was used")
	}
}

// Whisper skipped the first half of the take: what it did hear is right,
// but the start cannot be placed from it, so the timer refuses.
func TestTimerRefusesATranscriptMissingTheStart(t *testing.T) {
	text := []rune("uno dos tres cuatro cinco seis siete ocho nueve diez once doce trece catorce quince dieciseis diecisiete dieciocho diecinueve veinte")
	var words []Word
	for i, w := range strings.Fields(string(text))[8:] {
		words = append(words, Word{Text: w, Start: 22 + float64(i), End: 22.8 + float64(i)})
	}
	if _, ok := (Alignment{Words: words}).Timer(text)(10); ok {
		t.Error("placed a moment from a transcript that missed the first eight words")
	}
}
