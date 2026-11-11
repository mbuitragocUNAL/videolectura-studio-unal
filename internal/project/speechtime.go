package project

import (
	"math"
	"sort"
)

// TimeAt estimates when, in the recording, the reader reached a fraction of
// the paragraph's text.
//
// A take carries no word timings, only its loudness every twentieth of a
// second. So the text is spread evenly over the stretch that has speech in
// it, leaving out the silence at either end and whatever was cut, and the
// estimate is then moved to the nearest pause. A switch lands between
// sentences, where the reader breathes, rather than in the middle of a word.
func (t Take) TimeAt(frac float64) float64 {
	frac = math.Min(math.Max(frac, 0), 1)
	from, to := t.speech()

	var spans []Span
	total := 0.0
	for _, s := range KeptSpans(t.Cuts, t.Duration) {
		s.From, s.To = math.Max(s.From, from), math.Min(s.To, to)
		if s.To > s.From {
			spans = append(spans, s)
			total += s.Length()
		}
	}
	if total == 0 {
		return from + frac*(to-from)
	}
	target := frac * total
	at := spans[len(spans)-1].To
	for _, s := range spans {
		if target <= s.Length() {
			at = s.From + target
			break
		}
		target -= s.Length()
	}
	return t.nearestPause(at, 1.5)
}

// loud is the level speech sits at: the 90th percentile of the take.
func (t Take) loud() float64 {
	if len(t.Levels) == 0 {
		return 0
	}
	rs := make([]float64, len(t.Levels))
	for i, l := range t.Levels {
		rs[i] = l.RMS
	}
	sort.Float64s(rs)
	return rs[len(rs)*9/10]
}

// speech is where the voice starts and stops, or the whole take without levels.
func (t Take) speech() (float64, float64) {
	loud := t.loud()
	if loud <= 0 {
		return 0, t.Duration
	}
	from, to := -1.0, 0.0
	for _, l := range t.Levels {
		if l.RMS > loud/4 {
			if from < 0 {
				from = l.At
			}
			to = l.At
		}
	}
	if from < 0 || to <= from {
		return 0, t.Duration
	}
	return from, math.Min(to, t.Duration)
}

// nearestPause is the quietest moment within window seconds of at, with a
// small preference for staying close.
func (t Take) nearestPause(at, window float64) float64 {
	loud := t.loud()
	if loud <= 0 {
		return at
	}
	best, score := at, math.Inf(1)
	for i, l := range t.Levels {
		if math.Abs(l.At-at) > window {
			continue
		}
		sum, n := 0.0, 0
		for j := i; j >= 0 && l.At-t.Levels[j].At <= 0.15; j-- {
			sum += t.Levels[j].RMS
			n++
		}
		for j := i + 1; j < len(t.Levels) && t.Levels[j].At-l.At <= 0.15; j++ {
			sum += t.Levels[j].RMS
			n++
		}
		s := sum/float64(n)/loud + 0.08*math.Abs(l.At-at)
		if s < score {
			best, score = l.At, s
		}
	}
	return best
}
