package project

import (
	"strings"
	"unicode"
)

// Word is one word Whisper heard in a take, with when it starts and ends.
type Word struct {
	Text  string  `json:"w"`
	Start float64 `json:"a"`
	End   float64 `json:"b"`
}

// Alignment is a take's transcript, written by preparar/alinear.py.
type Alignment struct {
	Model string `json:"modelo"`
	Words []Word `json:"palabras"`
}

type token struct {
	norm       string
	from, to   int     // runes, in the expected text
	start, end float64 // seconds, for a heard token
}

// tokens splits text into words: letters and digits, lowercased, accents off.
func tokens(text []rune) []token {
	var out []token
	start := -1
	flush := func(end int) {
		if start >= 0 {
			out = append(out, token{norm: fold(string(text[start:end])), from: start, to: end})
			start = -1
		}
	}
	for i, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if start < 0 {
				start = i
			}
			continue
		}
		flush(i)
	}
	flush(len(text))
	return out
}

// unaccent covers what Spanish text and Whisper's spelling of it can differ in.
var unaccent = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
	"à", "a", "è", "e", "ì", "i", "ò", "o", "ù", "u", "ç", "c")

func fold(s string) string {
	return unaccent.Replace(strings.ToLower(s))
}

// alike is a forgiving match: Whisper writes «bakibol» for «buckyball» and
// «660» for «C60», and the words around them are what pin the timing.
func alike(a, b string) bool {
	if a == b {
		return true
	}
	if len(a) < 4 || len(b) < 4 {
		return false
	}
	n := min(len(a), len(b), 5)
	return a[:n-1] == b[:n-1]
}

// Timer turns a place in the text the reader read into the moment they read
// it. It matches the transcript's words to the text in order (a longest
// common subsequence, so words Whisper got wrong or invented are skipped) and
// puts each place in the pause before the word that follows it. ok is false
// when too little of the text was heard to trust.
func (a Alignment) Timer(text []rune) func(offset int) (float64, bool) {
	none := func(int) (float64, bool) { return 0, false }
	want := tokens(text)
	var heard []token
	for _, w := range a.Words {
		for _, t := range tokens([]rune(w.Text)) {
			t.start, t.end = w.Start, w.End
			heard = append(heard, t)
		}
	}
	if len(want) == 0 || len(heard) == 0 {
		return none
	}

	// lcs[i][j]: longest match of want[i:] against heard[j:].
	n, m := len(want), len(heard)
	lcs := make([][]int32, n+1)
	for i := range lcs {
		lcs[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case alike(want[i].norm, heard[j].norm):
				lcs[i][j] = lcs[i+1][j+1] + 1
			case lcs[i+1][j] >= lcs[i][j+1]:
				lcs[i][j] = lcs[i+1][j]
			default:
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	match := make([]int, n) // heard index for each wanted word, or -1
	for i := range match {
		match[i] = -1
	}
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case alike(want[i].norm, heard[j].norm) && lcs[i][j] == lcs[i+1][j+1]+1:
			match[i] = j
			i, j = i+1, j+1
		case lcs[i+1][j] >= lcs[i][j+1]:
			i++
		default:
			j++
		}
	}
	if int(lcs[0][0])*2 < n {
		return none
	}
	// A transcript that missed the start or the end of the take (Whisper
	// sometimes skips the first seconds) would place everything there by
	// interpolation, seconds off. Better the estimate than that.
	first, last := -1, -1
	for i, j := range match {
		if j >= 0 {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	slack := max(5, n/10)
	if first > slack || n-1-last > slack {
		return none
	}

	// When wanted word i ends and starts, from its own match or, failing
	// that, spread between the nearest matched words either side by length.
	type anchor struct {
		at   int
		time float64
	}
	var anchors []anchor
	for i, j := range match {
		if j >= 0 {
			anchors = append(anchors,
				anchor{want[i].from, heard[j].start}, anchor{want[i].to, heard[j].end})
		}
	}
	timeAt := func(at int) float64 {
		before, after := anchors[0], anchors[len(anchors)-1]
		for _, x := range anchors {
			if x.at <= at {
				before = x
			}
			if x.at >= at {
				after = x
				break
			}
		}
		if after.at == before.at {
			return before.time
		}
		f := float64(at-before.at) / float64(after.at-before.at)
		return before.time + f*(after.time-before.time)
	}
	startOf := func(i int) float64 {
		if match[i] >= 0 {
			return heard[match[i]].start
		}
		return timeAt(want[i].from)
	}
	endOf := func(i int) float64 {
		if match[i] >= 0 {
			return heard[match[i]].end
		}
		return timeAt(want[i].to)
	}

	return func(offset int) (float64, bool) {
		next := len(want)
		for i, t := range want {
			if t.from >= offset {
				next = i
				break
			}
		}
		switch {
		case next == 0:
			return startOf(0), true
		case next == len(want):
			return endOf(len(want) - 1), true
		}
		// In the pause between the two words, a breath before the next one,
		// so a new picture is up as the word that needs it is spoken.
		gapFrom, gapTo := endOf(next-1), startOf(next)
		if gapTo < gapFrom {
			gapFrom, gapTo = gapTo, gapFrom
		}
		return max(gapFrom, gapTo-0.12), true
	}
}
