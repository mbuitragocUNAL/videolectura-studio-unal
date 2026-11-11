package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Session is everything the studio accumulates while recording a reading: the
// takes themselves, where each quote fired inside them, what has been cut, and
// the order they are assembled in.
type Session struct {
	Takes     []Take    `json:"takes"`
	Timeline  Timeline  `json:"timeline"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Take is one recording. Cues and cuts are both measured in seconds from the
// start of the recording, which is also where the browser's clock started.
type Take struct {
	ID       int     `json:"id"`
	File     string  `json:"file"`
	Label    string  `json:"label"`
	Duration float64 `json:"duration"`
	Bytes    int64   `json:"bytes"`
	From     float64 `json:"from"` // strip position where the take opened
	To       float64 `json:"to"`   // and where it closed, 0..1
	// Which paragraph of the passage this take reads, counted from zero.
	//
	// It is recorded rather than derived from From, because at the very top of
	// the strip the page cannot scroll far enough to put the first paragraphs
	// under the reader's eye — the position clamps, and every take of the first
	// two paragraphs would look like a take of the third.
	Paragraph int `json:"paragraph"`
	// Plate marks the take that shows the lámina instead of the page. It is a
	// slot of its own beside the paragraph's reading, not a replacement for
	// it: the paragraph names the plate and then the plate is shown, so both
	// takes belong in the video and neither supersedes the other.
	Plate bool `json:"plate"`
	// Figure names which figure a plate take shows, for readings with more
	// than one. Empty for lectura 07's single lámina.
	Figure     string       `json:"figure,omitempty"`
	Cues       []Cue        `json:"cues"`
	Cuts       []Cut        `json:"cuts"`
	Levels     []Level      `json:"levels,omitempty"`
	Trace      []TracePoint `json:"trace,omitempty"`
	RecordedAt time.Time    `json:"recordedAt"`
}

// Cue marks the instant a quote was fired inside a take.
type Cue struct {
	Quote string  `json:"quote"`
	At    float64 `json:"at"`
}

// Cut is a stretch of a take to drop on render.
type Cut struct {
	From float64 `json:"from"`
	To   float64 `json:"to"`
}

// Length is how much time the cut removes.
func (c Cut) Length() float64 { return c.To - c.From }

// Level is one microphone reading captured while recording, kept so the
// waveform can be drawn without decoding the video.
type Level struct {
	At  float64 `json:"t"`
	RMS float64 `json:"r"`
}

// TracePoint records where the strip was at an instant of the take.
//
// Cues pin time to position at nineteen points; the trace pins it everywhere,
// which is what lets a cut be expressed as a line of the passage instead of a
// guess at a timestamp.
type TracePoint struct {
	At float64 `json:"t"`
	F  float64 `json:"f"`
}

// Timeline is the order the takes are assembled in. A take that is not on the
// timeline is kept on disk but left out of the render.
type Timeline struct {
	Entries []Entry `json:"entries"`
}

// Entry places one take on the timeline.
type Entry struct {
	TakeID int  `json:"takeId"`
	Muted  bool `json:"muted"`
}

// KeptDuration is the take's length once its cuts are applied.
func (t Take) KeptDuration() float64 {
	total := t.Duration
	for _, c := range MergeCuts(t.Cuts) {
		total -= c.Length()
	}
	if total < 0 {
		return 0
	}
	return total
}

// PositionAt is where the strip was, in 0..1, at the given second of the take.
// Between samples it interpolates; outside the trace it clamps to the ends.
func (t Take) PositionAt(seconds float64) (float64, bool) {
	if len(t.Trace) == 0 {
		return 0, false
	}
	if seconds <= t.Trace[0].At {
		return t.Trace[0].F, true
	}
	last := t.Trace[len(t.Trace)-1]
	if seconds >= last.At {
		return last.F, true
	}
	lo, hi := 0, len(t.Trace)-1
	for lo < hi {
		mid := (lo + hi) / 2
		if t.Trace[mid].At < seconds {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	b := t.Trace[lo]
	a := t.Trace[lo-1]
	span := b.At - a.At
	if span <= 0 {
		return b.F, true
	}
	return a.F + (b.F-a.F)*(seconds-a.At)/span, true
}

// Store holds a session on disk and serialises access to it. Every mutation
// goes through Update so concurrent requests cannot interleave a read-modify-
// write and lose each other's changes.
type Store struct {
	path string
	mu   sync.Mutex
	data Session
}

// OpenStore loads the session at dir/session.json, creating an empty one if it
// does not exist yet.
func OpenStore(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "session.json")}
	raw, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		s.data = Session{}
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", s.path, err)
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", s.path, err)
	}
	return s, nil
}

// Snapshot returns a copy of the session, safe to serve.
func (s *Store) Snapshot() Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.clone()
}

// Update applies fn to the session and persists the result. The callback runs
// under the lock, so it must not block.
func (s *Store) Update(fn func(*Session) error) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	draft := s.data.clone()
	if err := fn(&draft); err != nil {
		return Session{}, err
	}
	draft.UpdatedAt = time.Now().UTC()

	if err := writeJSONAtomic(s.path, draft); err != nil {
		return Session{}, err
	}
	s.data = draft
	return s.data.clone(), nil
}

// NextTakeID returns an id one past the highest in use, so ids stay stable even
// after a take is deleted.
func (s *Session) NextTakeID() int {
	next := 1
	for _, t := range s.Takes {
		if t.ID >= next {
			next = t.ID + 1
		}
	}
	return next
}

// TakeByID finds a take by id.
func (s *Session) TakeByID(id int) (*Take, bool) {
	for i := range s.Takes {
		if s.Takes[i].ID == id {
			return &s.Takes[i], true
		}
	}
	return nil, false
}

// OrderedTakes returns the takes in timeline order, skipping muted entries and
// any entry pointing at a take that no longer exists. A session whose timeline
// was never arranged falls back to recording order, which is the common case.
func (s *Session) OrderedTakes() []Take {
	if len(s.Timeline.Entries) == 0 {
		// In the order the passage runs, not the order they were recorded, and
		// one take per paragraph.
		//
		// A take is one paragraph. Reading a paragraph again after a stumble
		// leaves two takes of the same text, and putting both in the video
		// says the sentence twice; the later one is the keeper, because that
		// is what re-reading it meant. The earlier ones stay on disk so the
		// decision can be undone by deleting the newer.
		type slot struct {
			paragraph int
			plate     bool
			figure    string
		}
		newest := map[slot]Take{}
		for _, t := range s.Takes {
			key := slot{t.Paragraph, t.Plate, t.Figure}
			if seen, ok := newest[key]; ok && seen.ID > t.ID {
				continue
			}
			newest[key] = t
		}

		out := make([]Take, 0, len(newest))
		for _, t := range newest {
			// A take cut down to nothing is not a silent take, it is no take.
			if t.KeptDuration() <= 0 {
				continue
			}
			out = append(out, t)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Paragraph != out[j].Paragraph {
				return out[i].Paragraph < out[j].Paragraph
			}
			// The plate follows the paragraph that announces it.
			if out[i].Plate != out[j].Plate {
				return !out[i].Plate
			}
			if out[i].From != out[j].From {
				return out[i].From < out[j].From
			}
			return out[i].ID < out[j].ID
		})
		return out
	}
	var out []Take
	for _, e := range s.Timeline.Entries {
		if e.Muted {
			continue
		}
		if t, ok := s.TakeByID(e.TakeID); ok && t.KeptDuration() > 0 {
			out = append(out, *t)
		}
	}
	return out
}

func (s Session) clone() Session {
	raw, err := json.Marshal(s)
	if err != nil {
		return Session{}
	}
	var out Session
	if err := json.Unmarshal(raw, &out); err != nil {
		return Session{}
	}
	return out
}

// writeJSONAtomic writes through a temporary file and renames it, so a crash
// mid-write cannot leave a truncated session behind.
func writeJSONAtomic(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".session-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
