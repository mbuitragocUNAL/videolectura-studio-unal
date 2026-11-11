package project

import (
	"math"
	"sort"
)

// PauseRules say how much silence a take keeps in the render.
//
// A take is recorded between two key presses: there is a second or so of room
// tone before the reader starts and after they stop, and here and there a
// breath that ran long. Played back to back, two takes put two of those
// silences together, and a paragraph change becomes a gap of two seconds that
// nobody meant. The render cuts each end down to a natural margin and
// shortens the pauses that ran long, so the reading flows the way it was
// read, not the way it was recorded.
type PauseRules struct {
	Head     float64 // silence kept before the first word
	Tail     float64 // and after the last
	MaxPause float64 // a pause longer than this inside a take is shortened
	Keep     float64 // to this much each side of it (2*Keep in all)
}

// DefaultPauses keeps half a second between paragraphs (Tail + Head) and
// lets no pause inside one run past seven tenths.
var DefaultPauses = PauseRules{Head: 0.18, Tail: 0.32, MaxPause: 0.9, Keep: 0.35}

const (
	// Gaps shorter than this between loud frames are part of the word: the
	// stop before a «p», the hush between syllables.
	syllableGap = 0.15
	// A burst shorter than this is a click or a key, not speech.
	minSpeech = 0.08
)

// PauseCuts finds what to cut from a take, given its loudness in dB every
// step seconds. protect lists stretches that must stay whole, such as the
// silence a quote plays over; they count as speech, so a take that is only a
// quote is trimmed around the quote. Without a protected stretch it returns
// nil when it cannot tell voice from room tone, rather than guess.
func PauseCuts(db []float64, step, end float64, protect []Span, r PauseRules) []Cut {
	if len(db) < 10 || step <= 0 {
		return nil
	}
	sorted := append([]float64(nil), db...)
	sort.Float64s(sorted)
	floor := sorted[len(sorted)/10]
	voice := sorted[len(sorted)*9/10]
	threshold := math.Inf(1) // nothing is voice
	if voice-floor >= 12 {
		threshold = floor + 0.35*(voice-floor)
	} else if len(protect) == 0 {
		return nil
	}

	// Runs of speech, in frames, with syllable gaps closed and clicks dropped.
	type run struct{ from, to int }
	var runs []run
	for i := 0; i < len(db); i++ {
		if db[i] <= threshold {
			continue
		}
		j := i
		for j+1 < len(db) && db[j+1] > threshold {
			j++
		}
		if n := len(runs); n > 0 && float64(i-runs[n-1].to-1)*step < syllableGap {
			runs[n-1].to = j
		} else {
			runs = append(runs, run{i, j})
		}
		i = j
	}
	speech := runs[:0]
	for _, x := range runs {
		if float64(x.to-x.from+1)*step >= minSpeech {
			speech = append(speech, x)
		}
	}
	for _, p := range protect {
		from, to := max(0, int(p.From/step)), min(len(db)-1, int(math.Ceil(p.To/step)))
		if to >= from {
			speech = append(speech, run{from, to})
		}
	}
	if len(speech) == 0 {
		return nil
	}
	sort.Slice(speech, func(i, j int) bool { return speech[i].from < speech[j].from })
	merged := speech[:1]
	for _, x := range speech[1:] {
		if last := &merged[len(merged)-1]; x.from <= last.to+1 {
			last.to = max(last.to, x.to)
		} else {
			merged = append(merged, x)
		}
	}
	speech = merged

	at := func(frame int) float64 { return math.Min(end, float64(frame)*step) }
	var cuts []Cut
	add := func(from, to float64) {
		if to-from < 0.05 {
			return
		}
		for _, p := range protect {
			if from < p.To && to > p.From {
				return
			}
		}
		cuts = append(cuts, Cut{From: from, To: to})
	}

	first, last := speech[0], speech[len(speech)-1]
	add(0, at(first.from)-r.Head)
	add(at(last.to+1)+r.Tail, end)
	if r.MaxPause > 0 {
		for k := 1; k < len(speech); k++ {
			from, to := at(speech[k-1].to+1), at(speech[k].from)
			if to-from > r.MaxPause {
				add(from+r.Keep, to-r.Keep)
			}
		}
	}
	return cuts
}
