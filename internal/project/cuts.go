package project

import "sort"

// minSpan drops the slivers a cut leaves behind.
//
// Eighty milliseconds, not the twenty it used to be, because of what happens
// downstream: the render fades fifteen milliseconds into each end of a span,
// and only bothers when the span is longer than four of those fades. A
// thirty-millisecond remnant therefore went into the video as a hard splice at
// both ends, which is a click — and a span that short is never a piece of
// speech anybody meant to keep.
const minSpan = 0.08

// Span is a stretch of a take that survives its cuts.
type Span struct {
	From float64
	To   float64
}

// Length is the span's duration.
func (s Span) Length() float64 { return s.To - s.From }

// MergeCuts sorts the cuts and merges any that overlap or touch, so the rest of
// the pipeline can assume they are disjoint and in order.
func MergeCuts(cuts []Cut) []Cut {
	if len(cuts) == 0 {
		return nil
	}
	sorted := append([]Cut(nil), cuts...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].From < sorted[j].From })

	var merged []Cut
	for _, c := range sorted {
		if c.To <= c.From {
			continue
		}
		if n := len(merged); n > 0 && c.From <= merged[n-1].To {
			if c.To > merged[n-1].To {
				merged[n-1].To = c.To
			}
			continue
		}
		merged = append(merged, c)
	}
	return merged
}

// KeptSpans is the complement of the cuts over [0, end]: what actually survives
// into the render, in order.
func KeptSpans(cuts []Cut, end float64) []Span {
	var spans []Span
	cursor := 0.0
	for _, c := range MergeCuts(cuts) {
		if c.From > cursor {
			to := c.From
			if to > end {
				to = end
			}
			spans = append(spans, Span{From: cursor, To: to})
		}
		if c.To > cursor {
			cursor = c.To
		}
	}
	if cursor < end {
		spans = append(spans, Span{From: cursor, To: end})
	}

	kept := spans[:0]
	for _, s := range spans {
		if s.Length() > minSpan {
			kept = append(kept, s)
		}
	}
	return kept
}

// RetimeCues shifts every cue left by the cuts that precede it and reports the
// ones that fall inside a cut, which cannot survive.
//
// Removing four seconds from the middle of a take moves everything after it, so
// a cue left at its original time would land on the wrong words.
func RetimeCues(cues []Cue, cuts []Cut) (kept []Cue, dropped []Cue) {
	merged := MergeCuts(cuts)
	for _, cue := range cues {
		inside := false
		removed := 0.0
		for _, c := range merged {
			switch {
			case cue.At >= c.From && cue.At <= c.To:
				inside = true
			case c.To <= cue.At:
				removed += c.Length()
			}
		}
		if inside {
			dropped = append(dropped, cue)
			continue
		}
		kept = append(kept, Cue{Quote: cue.Quote, At: round3(cue.At - removed)})
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].At < kept[j].At })
	return kept, dropped
}

func round3(v float64) float64 {
	return float64(int64(v*1000+0.5)) / 1000
}
