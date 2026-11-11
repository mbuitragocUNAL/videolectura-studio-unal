package project

import (
	"math"
	"testing"
)

// Lectura 22's buckyball paragraph names Figura 27 and then Figura 30. Both
// are fixed over the whole paragraph, so picking the one with most overlap
// kept Figura 27 up and Figura 30 never appeared beside the text.
func TestSideFiguresSwitchAtTheSentenceThatNamesTheNext(t *testing.T) {
	r := &Reading{
		Lines: []Line{
			{Number: 1, Text: "     Cierra una hoja. La unidad de la Figura 27 (a la derecha)"},
			{Number: 2, Text: "se puede usar. La bonita molécula C60, mostrada en la Figura 30,"},
			{Number: 3, Text: "es particularmente estable."},
			{Number: 4, Text: "FIGURA 30. Estructura de la buckyball.", Kind: "pie"},
		},
		Figures: []Figure{{ID: "figura-27", Name: "Figura 27"}, {ID: "figura-30", Name: "Figura 30"}},
		Fixed: []FixedFigure{
			{Figure: "figura-30", From: 1, To: 3},
			{Figure: "figura-27", From: 1, To: 3},
		},
	}
	parts := r.SideFigures(r.Paragraphs()[0])
	if len(parts) != 2 || parts[0].Figure != "figura-27" || parts[1].Figure != "figura-30" {
		t.Fatalf("got %+v, want Figura 27 then Figura 30", parts)
	}
	if parts[0].At != 0 {
		t.Errorf("the first figure starts at %v, want 0", parts[0].At)
	}
	text := "Cierra una hoja. La unidad de la Figura 27 (a la derecha) se puede usar. La bonita molécula C60, mostrada en la Figura 30, es particularmente estable."
	want := float64(len([]rune("Cierra una hoja. La unidad de la Figura 27 (a la derecha) se puede usar. "))) /
		float64(len([]rune(text)))
	if math.Abs(parts[1].At-want) > 1e-9 {
		t.Errorf("Figura 30 takes over at %v, want %v (the start of «La bonita molécula»)", parts[1].At, want)
	}
}

// A figure carried over from the paragraph before is already on screen, so it
// stays first even when this paragraph names it after the new one.
func TestSideFiguresKeepsACarriedFigureFirst(t *testing.T) {
	r := &Reading{
		Lines: []Line{
			{Number: 1, Text: "     Antes."},
			{Number: 2, Text: "     Mira la Figura 9. Y vuelve a la Figura 8."},
		},
		Figures: []Figure{{ID: "f8", Name: "Figura 8"}, {ID: "f9", Name: "Figura 9"}},
		Fixed: []FixedFigure{
			{Figure: "f8", From: 1, To: 2},
			{Figure: "f9", From: 2, To: 2},
		},
	}
	parts := r.SideFigures(r.Paragraphs()[1])
	if len(parts) != 2 || parts[0].Figure != "f8" || parts[1].Figure != "f9" {
		t.Fatalf("got %+v, want f8 (carried) then f9", parts)
	}
}

// «Figura 3» must not be found inside «Figura 30».
func TestMentionNeedsAWholeNumber(t *testing.T) {
	if i := mention([]rune("ver la Figura 30."), "Figura 3", 0); i != -1 {
		t.Errorf("found Figura 3 at %d inside Figura 30", i)
	}
	if i := mention([]rune("ver la FIGURA 3."), "Figura 3", 0); i != 7 {
		t.Errorf("found Figura 3 at %d, want 7", i)
	}
}

// The switch moves to the pause between sentences rather than landing where
// an even spread of the text puts it.
func TestTimeAtSnapsToThePause(t *testing.T) {
	take := Take{Duration: 10}
	for at := 0.0; at < 10; at += 0.05 {
		r := 0.04
		switch {
		case at < 1 || at > 9: // silence before and after the reading
			r = 0.002
		case at > 5.6 && at < 6.1: // the breath between sentences
			r = 0.003
		}
		take.Levels = append(take.Levels, Level{At: at, RMS: r})
	}
	// Half the text, spread over 1..9 s of speech, is 5 s; the pause is at 5.6..6.1.
	got := take.TimeAt(0.5)
	if got < 5.6 || got > 6.1 {
		t.Errorf("switch at %.2f s, want inside the pause at 5.6–6.1 s", got)
	}
	// Without levels it is a straight proportion of the take.
	if got := (Take{Duration: 10}).TimeAt(0.25); got != 2.5 {
		t.Errorf("without levels got %.2f s, want 2.5", got)
	}
}

// Lineamiento 2.d, with the caption read inside the paragraph's take: the
// page until the figure is named, the figure with its caption while the
// caption is read, a hold, then the page again with the arrow on the line the
// reading resumes at. The figure only stays on the right after its caption.
func TestBeatsPlaceTheCaptionWhereItIsRead(t *testing.T) {
	r := &Reading{
		Lines: []Line{
			{Number: 1, Text: "     Hay dos formas bonitas, en la Figura 27. Y luego"},
			{Number: 2, Text: "sigue el texto."},
			{Number: 3, Text: "FIGURA 27. Dos patrones.", Kind: "pie"},
		},
		Figures: []Figure{{ID: "figura-27", Name: "Figura 27", Caption: "FIGURA 27. Dos patrones."}},
		Events:  []FigureEvent{{Figure: "figura-27", Mode: ModeCaption, Line: 1}},
		Fixed:   []FixedFigure{{Figure: "figura-27", From: 1, To: 2}},
	}
	beats, said, toSaid := r.Beats(r.Paragraphs()[0], nil)
	kinds := ""
	for _, b := range beats {
		kinds += b.Kind + ":" + b.Figure + " "
	}
	want := "pagina: pie:figura-27 pausa:figura-27 pagina:figura-27 "
	if kinds != want {
		t.Fatalf("beats %q, want %q", kinds, want)
	}
	if beats[3].Resume != 1 {
		t.Errorf("the arrow points at line %d, want 1, where «Y luego» resumes", beats[3].Resume)
	}
	if !beats[2].WithCaption {
		t.Error("the hold after a read caption keeps the caption")
	}
	// The caption is read after «Figura 27.»: 44 runes in, 24 long, of 75 spoken.
	text := "Hay dos formas bonitas, en la Figura 27. Y luego sigue el texto."
	spoken := len([]rune(text)) + 1 + len([]rune("FIGURA 27. Dos patrones."))
	at := float64(len([]rune("Hay dos formas bonitas, en la Figura 27."))) / float64(spoken)
	if beats[1].From != at {
		t.Errorf("caption starts at %v, want %v", beats[1].From, at)
	}
	if want := "Hay dos formas bonitas, en la Figura 27. FIGURA 27. Dos patrones. Y luego sigue el texto."; string(said) != want {
		t.Errorf("said %q, want %q", string(said), want)
	}
	// «Y luego» is at 41 in the printed text; in what was said the caption
	// came first.
	if got, want := toSaid(41), 41+1+len([]rune("FIGURA 27. Dos patrones.")); got != want {
		t.Errorf("toSaid(41) = %d, want %d", got, want)
	}
	if got := toSaid(10); got != 10 {
		t.Errorf("before the caption toSaid(10) = %d, want 10", got)
	}

	// Recorded as a take of its own, the caption comes after the paragraph,
	// and so does the figure on the right.
	if got, _, _ := r.Beats(r.Paragraphs()[0], map[string]bool{"figura-27": true}); len(got) != 1 || got[0].Figure != "" {
		t.Errorf("with the caption recorded apart got %+v, want the plain page", got)
	}
}
