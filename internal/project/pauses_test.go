package project

import (
	"math"
	"testing"
)

// levels builds a take's loudness every 20 ms: room tone at -60 dB, speech at
// -20 dB over the given stretches.
func levels(end float64, speech ...[2]float64) []float64 {
	var db []float64
	for t := 0.0; t < end; t += 0.02 {
		v := -60.0
		for _, s := range speech {
			if t >= s[0] && t < s[1] {
				v = -20
			}
		}
		db = append(db, v)
	}
	return db
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.03 }

func TestPauseCutsTrimTheEndsAndTheLongPause(t *testing.T) {
	// 1.4 s of room tone, speech, a 2 s pause, speech, 1.1 s of room tone.
	db := levels(10, [2]float64{1.4, 4.0}, [2]float64{6.0, 8.9})
	cuts := PauseCuts(db, 0.02, 10, nil, DefaultPauses)
	if len(cuts) != 3 {
		t.Fatalf("got %+v, want head, tail and the long pause", cuts)
	}
	head, tail, pause := cuts[0], cuts[1], cuts[2]
	if head.From != 0 || !near(head.To, 1.4-0.18) {
		t.Errorf("head %+v, want 0..1.22", head)
	}
	if !near(tail.From, 8.9+0.32) || tail.To != 10 {
		t.Errorf("tail %+v, want 9.22..10", tail)
	}
	if !near(pause.From, 4.35) || !near(pause.To, 5.65) {
		t.Errorf("pause %+v, want 4.35..5.65, leaving 0.7 s", pause)
	}
}

func TestPauseCutsLeaveShortPausesAndSyllablesAlone(t *testing.T) {
	// A 0.6 s breath and a 0.1 s stop inside a word: both stay.
	db := levels(6, [2]float64{0.2, 2.0}, [2]float64{2.6, 3.5}, [2]float64{3.6, 5.8})
	for _, c := range PauseCuts(db, 0.02, 6, nil, DefaultPauses) {
		if c.From > 0.5 && c.To < 5.5 {
			t.Errorf("cut %+v inside the reading", c)
		}
	}
}

// The silence a quote plays over is the quote's place in the take.
func TestPauseCutsKeepTheGapAQuoteFills(t *testing.T) {
	db := levels(12, [2]float64{0.5, 3.0}, [2]float64{8.0, 11.5})
	quote := []Span{{From: 3.2, To: 7.8}}
	for _, c := range PauseCuts(db, 0.02, 12, quote, DefaultPauses) {
		if c.From < 7.8 && c.To > 3.2 {
			t.Errorf("cut %+v swallows the quote", c)
		}
	}
}

// A key press before the reader starts is not where the reading starts.
func TestPauseCutsIgnoreAClick(t *testing.T) {
	db := levels(6, [2]float64{0.3, 0.34}, [2]float64{1.5, 5.5})
	cuts := PauseCuts(db, 0.02, 6, nil, DefaultPauses)
	if len(cuts) == 0 || !near(cuts[0].To, 1.5-0.18) {
		t.Errorf("got %+v, want the head cut up to the speech at 1.5 s", cuts)
	}
}

// Without a clear difference between voice and room, nothing is cut.
func TestPauseCutsNeedToHearTheVoice(t *testing.T) {
	db := make([]float64, 300)
	for i := range db {
		db[i] = -40 + float64(i%5)
	}
	if cuts := PauseCuts(db, 0.02, 6, nil, DefaultPauses); cuts != nil {
		t.Errorf("got %+v from a take with no voice to find", cuts)
	}
}

// A take that is only a quote (the reader silent, the clip in the headphones)
// has no voice to find; it is trimmed around the quote instead.
func TestPauseCutsTrimAroundAQuoteAlone(t *testing.T) {
	db := make([]float64, 240) // 4.8 s of digital silence
	for i := range db {
		db[i] = -120
	}
	quote := []Span{{From: 0.5, To: 3.7}}
	cuts := PauseCuts(db, 0.02, 4.8, quote, DefaultPauses)
	if len(cuts) != 2 || !near(cuts[0].To, 0.5-0.18) || !near(cuts[1].From, 3.7+0.32) {
		t.Fatalf("got %+v, want the head up to 0.32 s and the tail from 4.02 s", cuts)
	}
}
