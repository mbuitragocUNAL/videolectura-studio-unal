package render

import (
	"math"
	"strings"
	"testing"

	"studio/internal/project"
)

// The inputs are always the page first, then the recording, then a quote each.
// Every label in the graph depends on that order.
func TestBuildArgsOrdersInputsPageThenVoice(t *testing.T) {
	a := BuildArgs(TakePlan{
		Source: "take.webm", Frame: "page.png", Duration: 30, Height: 1080,
	})

	if len(a.Inputs) != 2 || a.Inputs[0] != "page.png" || a.Inputs[1] != "take.webm" {
		t.Fatalf("inputs %v", a.Inputs)
	}
	// The picture is the drawn page, never the recording's own video.
	if !strings.Contains(a.Filter, "[0:v]scale=1920:1080,setsar=1[v]") {
		t.Fatalf("filter %q", a.Filter)
	}
	if strings.Contains(a.Filter, "[1:v]") {
		t.Fatalf("the recording's video is being used: %q", a.Filter)
	}
	// A bare input stream is mapped as `1:a`; only a filter output takes
	// brackets. ffmpeg rejects `-map [1:a]` outright.
	if a.MapA != "[ap]" || !strings.Contains(a.Filter, "[1:a]apad[ap]") {
		t.Fatalf("mapA %q filter %q, want the microphone padded", a.MapA, a.Filter)
	}
}

// The piece runs exactly as long as its sound. -shortest used to end it at
// whichever stream ran out first, and the captured video ran out early.
func TestBuildArgsRunsForTheLengthOfTheAudio(t *testing.T) {
	a := BuildArgs(TakePlan{
		Source: "take.webm", Frame: "page.png", Duration: 30, Height: 1080,
		Cuts: []project.Cut{{From: 10, To: 20}},
	})
	if a.Seconds != 20 {
		t.Fatalf("seconds %v, want 20", a.Seconds)
	}

	joined := strings.Join(a.CommandLine("out.mp4"), " ")
	if !strings.Contains(joined, "-t 20.000") {
		t.Fatalf("length not pinned: %q", joined)
	}
	if strings.Contains(joined, "-shortest") {
		t.Fatalf("-shortest is back: %q", joined)
	}
	if !strings.Contains(joined, "-loop 1 -framerate 25 -i page.png") {
		t.Fatalf("the still is not looped: %q", joined)
	}
}

func TestBuildArgsPlacesQuotes(t *testing.T) {
	a := BuildArgs(TakePlan{
		Source: "take.webm", Frame: "page.png", Duration: 60, Height: 1080, Gain: 1,
		Cues: []QuoteCue{{File: "a.mp3", At: 2}, {File: "b.mp3", At: 12.5}},
	})

	if len(a.Inputs) != 4 {
		t.Fatalf("inputs %v", a.Inputs)
	}
	if !strings.Contains(a.Filter, "[2:a]adelay=2000|2000") {
		t.Fatalf("first quote not delayed to 2s: %q", a.Filter)
	}
	if !strings.Contains(a.Filter, "[3:a]adelay=12500|12500") {
		t.Fatalf("second quote not delayed to 12.5s: %q", a.Filter)
	}
	if !strings.Contains(a.Filter, "amix=inputs=3:normalize=0:dropout_transition=0:duration=first") {
		t.Fatalf("mix is wrong: %q", a.Filter)
	}
	if a.MapA != "[ap]" || !strings.Contains(a.Filter, "[a]apad[ap]") {
		t.Fatalf("mapA %q filter %q, want the mix padded", a.MapA, a.Filter)
	}
}

// Only the sound is spliced now: a still needs no cutting.
func TestBuildArgsSplicesAudioOnly(t *testing.T) {
	a := BuildArgs(TakePlan{
		Source: "take.webm", Frame: "page.png", Duration: 60, Height: 1080, Gain: 1,
		Cuts: []project.Cut{{From: 10, To: 20}},
	})

	for _, want := range []string{
		"[1:a]atrim=start=0.000:end=10.000",
		"[1:a]atrim=start=20.000:end=60.000",
		"concat=n=2:v=0:a=1[ac]",
	} {
		if !strings.Contains(a.Filter, want) {
			t.Fatalf("missing %q in %q", want, a.Filter)
		}
	}
	if strings.Contains(a.Filter, "trim=start") && strings.Contains(a.Filter, "[0:v]trim") {
		t.Fatalf("the page is being trimmed: %q", a.Filter)
	}
	if a.MapA != "[ap]" || !strings.Contains(a.Filter, "[ac]apad[ap]") {
		t.Fatalf("mapA %q filter %q, want the spliced sound padded", a.MapA, a.Filter)
	}
}

func TestBuildArgsFadesEverySplice(t *testing.T) {
	a := BuildArgs(TakePlan{
		Source: "take.webm", Frame: "page.png", Duration: 60, Height: 1080,
		Cuts: []project.Cut{{From: 10, To: 20}},
	})
	if n := strings.Count(a.Filter, "afade=t=in"); n != 2 {
		t.Fatalf("expected a fade in on each of the 2 spans, got %d: %q", n, a.Filter)
	}
	if n := strings.Count(a.Filter, "afade=t=out"); n != 2 {
		t.Fatalf("expected a fade out on each of the 2 spans, got %d", n)
	}
}

// Every span that survives a cut can carry its fades.
//
// KeptSpans drops anything under 80 ms and a fade needs 60, so the branch in
// BuildArgs that skips fading a sliver is now unreachable from real cuts. It
// stays as a guard on a function that does not get to assume its caller
// filtered; this pins the relationship that makes it moot.
func TestEverySurvivingSpanIsLongEnoughToFade(t *testing.T) {
	spans := project.KeptSpans([]project.Cut{{From: 0.09, To: 20}}, 60)
	for _, s := range spans {
		if s.Length() <= 4*edgeFade {
			t.Fatalf("a span of %.3fs survived but cannot be faded", s.Length())
		}
	}

	a := BuildArgs(TakePlan{
		Source: "take.webm", Frame: "page.png", Duration: 60, Height: 1080,
		Cuts: []project.Cut{{From: 0.09, To: 20}},
	})
	if n := strings.Count(a.Filter, "afade=t=in"); n != len(spans) {
		t.Fatalf("faded %d of %d spans: %q", n, len(spans), a.Filter)
	}
}

func TestCommandLineShape(t *testing.T) {
	a := BuildArgs(TakePlan{Source: "take.webm", Frame: "page.png", Duration: 10, Height: 1080})
	cmd := a.CommandLine("out.mp4")

	joined := strings.Join(cmd, " ")
	for _, want := range []string{
		"-i page.png", "-i take.webm", "-map [v]", "-map [ap]", "-c:a pcm_s16le",
		"-c:v libx264", "-pix_fmt yuv420p", "out.mp4",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
	if cmd[len(cmd)-1] != "out.mp4" {
		t.Fatalf("output must come last: %v", cmd)
	}
}

func TestBuildArgsCutsAndQuotesTogether(t *testing.T) {
	a := BuildArgs(TakePlan{
		Source: "take.webm", Frame: "page.png", Duration: 60, Height: 1080, Gain: 1.3,
		Cuts: []project.Cut{{From: 10, To: 20}},
		Cues: []QuoteCue{{File: "a.mp3", At: 25}},
	})
	// The mix must draw the voice from the spliced stream, not the raw input.
	if !strings.Contains(a.Filter, "[ac][c2]amix=") {
		t.Fatalf("mix does not consume the concat output: %q", a.Filter)
	}
	if !strings.Contains(a.Filter, "volume=1.3") {
		t.Fatalf("gain not applied: %q", a.Filter)
	}
}

// A take whose cuts remove all of it used to slip past the branch that applies
// cuts and be encoded whole — the reader heard back the very passage they had
// just deleted.
func TestBuildArgsRefusesATakeWithNothingLeft(t *testing.T) {
	args := BuildArgs(TakePlan{
		Source: "take.webm", Frame: "page.png", Duration: 6,
		Cuts: []project.Cut{{From: 0, To: 6}}, Height: 1080,
	})

	if !args.Empty {
		t.Fatalf("a fully cut take built a real graph: %q", args.Filter)
	}
	if strings.Contains(args.Filter, "scale") {
		t.Fatalf("a fully cut take still scales video: %q", args.Filter)
	}
}

// The lámina can go in without anybody having read it: the plate held over
// generated silence. Input 1 is then a filter, not a file.
func TestBuildArgsHoldsAPictureOverSilence(t *testing.T) {
	a := BuildArgs(TakePlan{
		Frame: "lamina.png", Fit: true, Silent: true, Duration: 3, Height: 1080,
	})

	if len(a.Inputs) != 2 || a.Inputs[1] != "anullsrc=channel_layout=stereo:sample_rate=48000" {
		t.Fatalf("inputs %v", a.Inputs)
	}
	if !strings.Contains(a.Filter, "force_original_aspect_ratio=decrease") {
		t.Fatalf("the plate is not letterboxed: %q", a.Filter)
	}

	joined := strings.Join(a.CommandLine("out.mp4"), " ")
	if !strings.Contains(joined, "-f lavfi -i anullsrc") {
		t.Fatalf("silence is not generated: %q", joined)
	}
	if !strings.Contains(joined, "-t 3.0000") {
		t.Fatalf("the hold is not bounded: %q", joined)
	}
	// The picture is still looped; only its sound is invented.
	if !strings.Contains(joined, "-loop 1 -framerate 25 -i lamina.png") {
		t.Fatalf("the plate is not looped: %q", joined)
	}
}

// Takes recorded on different days came in 14 dB apart. Each is brought to
// the same loudness before anything is mixed into it.
func TestBuildArgsLevelsTheNarration(t *testing.T) {
	args := BuildArgs(TakePlan{Source: "t.webm", Frame: "p.png", Duration: 10, Height: 1080, Level: 13.5})
	if !strings.Contains(args.Filter, "[1:a]volume=13.50dB,alimiter=") || !strings.Contains(args.Filter, "[lv]apad[ap]") {
		t.Fatalf("filter %q maps %q, want the voice raised 13.5 dB and limited", args.Filter, args.MapA)
	}
	silent := BuildArgs(TakePlan{Frame: "p.png", Duration: 3, Height: 1080, Silent: true, Level: 5})
	if strings.Contains(silent.Filter, "volume=") {
		t.Errorf("a silent hold was levelled: %q", silent.Filter)
	}
}

func TestParseIntegratedReadsTheSummary(t *testing.T) {
	log := "[Parsed_ebur128_0] t: 1 I: -40.0 LUFS\n[Parsed_ebur128_0] Summary:\n\n  Integrated loudness:\n    I:         -31.5 LUFS\n    Threshold: -42.0 LUFS\n"
	got, err := parseIntegrated(log)
	if err != nil || got != -31.5 {
		t.Fatalf("got %v, %v; want -31.5", got, err)
	}
}

// A piece lasts a whole number of frames, so joined pieces cannot drift.
func TestPiecesLastWholeFrames(t *testing.T) {
	for _, c := range []struct{ in, want float64 }{{7.221, 7.24}, {7.239, 7.24}, {0.001, 0.04}, {10, 10}} {
		if got := FrameLength(c.in); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("FrameLength(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
