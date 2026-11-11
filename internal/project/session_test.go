package project

import "testing"

// A take is one paragraph of the passage. Re-reading a paragraph that came out
// badly produces a new take with a higher id but the same place in the text,
// and the video has to follow the text.
func TestOrderedTakesFollowsThePassage(t *testing.T) {
	sess := Session{Takes: []Take{
		{ID: 1, Paragraph: 0, From: 0.10, Duration: 5},
		{ID: 2, Paragraph: 2, From: 0.30, Duration: 5},
		{ID: 3, Paragraph: 1, From: 0.20, Duration: 5},
	}}

	got := sess.OrderedTakes()
	want := []int{1, 3, 2}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("take %d of the render is %d, want %d", i, got[i].ID, id)
		}
	}
}

// Reading a paragraph twice must not say it twice: the later take is the one
// the reader meant to keep, and the earlier one stays on disk but out of the
// video.
func TestOrderedTakesKeepsOnlyTheNewestOfAParagraph(t *testing.T) {
	sess := Session{Takes: []Take{
		{ID: 4, Paragraph: 3, Duration: 5},
		{ID: 7, Paragraph: 3, Duration: 5},
	}}

	got := sess.OrderedTakes()
	if len(got) != 1 || got[0].ID != 7 {
		t.Fatalf("got %d takes starting with %d; want 1, id 7", len(got), got[0].ID)
	}
}

// A take cut down to nothing was cut down to nothing on purpose. Before, it
// fell past the branch that applies cuts and was rendered whole.
func TestOrderedTakesDropsATakeWithNothingLeft(t *testing.T) {
	sess := Session{Takes: []Take{
		{ID: 1, Paragraph: 0, Duration: 5},
		{ID: 2, Paragraph: 1, Duration: 7, Cuts: []Cut{{From: 0, To: 7}}},
	}}

	got := sess.OrderedTakes()
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("got %d takes; want only take 1", len(got))
	}
}

// The lámina is a take of its own, sitting between the paragraph that
// announces it and the next one. It must not be mistaken for a retake of that
// paragraph, or showing the plate would silently delete the reading.
func TestOrderedTakesKeepsThePlateBesideItsParagraph(t *testing.T) {
	sess := Session{Takes: []Take{
		{ID: 1, Paragraph: 15, Duration: 5},
		{ID: 2, Paragraph: 15, Duration: 4, Plate: true},
		{ID: 3, Paragraph: 16, Duration: 5},
	}}

	got := sess.OrderedTakes()
	if len(got) != 3 {
		t.Fatalf("got %d takes, want 3 — the plate displaced the reading", len(got))
	}
	for i, want := range []int{1, 2, 3} {
		if got[i].ID != want {
			t.Fatalf("take %d is %d, want %d", i, got[i].ID, want)
		}
	}
	if !got[1].Plate {
		t.Fatalf("the plate is not second")
	}
}

// A second attempt at the plate still supersedes the first.
func TestOrderedTakesKeepsTheNewestPlate(t *testing.T) {
	sess := Session{Takes: []Take{
		{ID: 1, Paragraph: 15, Duration: 5, Plate: true},
		{ID: 2, Paragraph: 15, Duration: 5, Plate: true},
	}}
	got := sess.OrderedTakes()
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("got %d takes starting with %d; want 1, id 2", len(got), got[0].ID)
	}
}
