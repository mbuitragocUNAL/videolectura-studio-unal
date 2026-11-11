// Package render turns a recorded session into the delivered video: it builds
// the ffmpeg filter graph for each take and runs the jobs.
package render

import (
	"fmt"
	"math"
	"strings"

	"studio/internal/project"
)

// edgeFade is applied at both ends of every kept span. A hard splice snaps the
// waveform and that snap is audible as a click; 15 ms of fade removes it
// without shifting any timing.
const edgeFade = 0.015

// TakePlan is everything needed to render one take into a piece.
type TakePlan struct {
	Source string // the recording; only its microphone track is used
	// Frame is the page to show for the whole take, drawn from the book's own
	// images. The recording's video is not used at all — see BuildArgs.
	Frame string
	// Fit letterboxes the picture instead of filling the frame. The page crop
	// is cut to 16:9 and fills it exactly; the lámina is whatever shape the
	// plate is, and cropping it to fit would take the edges off a picture the
	// rules say to show whole.
	Fit bool
	// Silent means there is no recording behind this piece: the picture is
	// held over silence for Duration. The lámina can be shown this way
	// without anybody reading anything, which is the cheap version of it.
	Silent   bool
	Output   string        // path to write
	Duration float64       // the take's full length, before cuts
	Cuts     []project.Cut // stretches to drop
	Cues     []QuoteCue    // already retimed against the cuts
	Gain     float64       // quote volume relative to the narration
	// Level is the gain applied to the narration, in dB, so every take sits
	// at the same loudness. Set by the runner from Plan.Loudness.
	Level  float64
	Height int // output height; the width is 16:9 of it
	// Scroll, when set, makes the picture the typeset column moving with the
	// reading, over Frame as its background.
	Scroll *Scroll
}

// Kept is how long the piece runs once the cuts are applied.
func (p TakePlan) Kept() float64 {
	total := p.Duration
	for _, c := range project.MergeCuts(p.Cuts) {
		total -= c.Length()
	}
	if total < 0 {
		return 0
	}
	return total
}

// QuoteCue is a quote placed at an instant of the rendered piece.
type QuoteCue struct {
	File string
	At   float64
}

// Args is a fully built ffmpeg invocation.
type Args struct {
	// Inputs in order: 0 is the still page, 1 is the recording, then a quote
	// each. The page is looped, so its input carries -loop and -framerate.
	Inputs  []string
	Filter  string
	MapV    string
	MapA    string
	Seconds float64 // how long to run; the still would otherwise loop forever
	Silent  bool    // input 1 is generated silence rather than a recording
	// Empty means the cuts left nothing of this take, so there is nothing to
	// encode. Rendering it anyway would put the whole uncut take in the video.
	Empty bool
}

/*
BuildArgs assembles the filter graph for one take.

The picture is a still of the book page, held for the whole piece, and the
sound is the take's microphone with its cuts taken out and its quotes mixed in.
The recording's own video is not used.

That is not a stylistic choice. A browser only hands over a frame when the tab
changes, and the page is deliberately motionless while a paragraph is read, so
the captures came out at one frame per second — twenty-one minutes of speech
carrying fourteen minutes of pictures, drifting apart from the first splice.
Drawing the page instead makes the two exact by construction: the video is
generated to the length of the audio rather than hoping to match it.

It also removes the old video trim-and-concat entirely. A still does not need
cutting, so only the sound is spliced.
*/
func BuildArgs(p TakePlan) Args {
	// A take whose cuts remove everything is not a silent take, it is no take.
	spans := project.KeptSpans(p.Cuts, p.Duration)
	if len(project.MergeCuts(p.Cuts)) > 0 && len(spans) == 0 {
		return Args{Empty: true}
	}

	// 0 is the page, 1 is the recording, then a quote each. A silent piece has
	// no recording, so input 1 is generated hush instead.
	inputs := []string{p.Frame, p.Source}
	if p.Silent {
		inputs = []string{p.Frame, "anullsrc=channel_layout=stereo:sample_rate=48000"}
	}
	for _, c := range p.Cues {
		inputs = append(inputs, c.File)
	}

	width := p.Height * 16 / 9
	var parts []string
	if p.Scroll != nil {
		column, arrow := len(inputs), -1
		inputs = append(inputs, p.Scroll.Column)
		if p.Scroll.Arrow != "" {
			arrow = len(inputs)
			inputs = append(inputs, p.Scroll.Arrow)
		}
		parts = append(parts, scrollGraph(p.Scroll, column, arrow, width, p.Height))
	} else {
		picture := fmt.Sprintf("scale=%d:%d", width, p.Height)
		if p.Fit {
			picture = fmt.Sprintf(
				"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:'(ow-iw)/2':'(oh-ih)/2':white",
				width, p.Height, width, p.Height)
		}
		parts = append(parts, fmt.Sprintf("[0:v]%s,setsar=1[v]", picture))
	}

	// `voice` names the sound inside the graph; `mapA` names it to -map, and
	// the two spellings differ. A bare stream is `1:a` on the command line but
	// `[1:a]` inside a filter, and handing ffmpeg the bracketed form as a map
	// makes it look for a filter output that was never defined.
	voice := "[1:a]"
	mapA := "1:a"
	if len(project.MergeCuts(p.Cuts)) > 0 {
		var chain strings.Builder
		for i, s := range spans {
			audio := fmt.Sprintf("[1:a]atrim=start=%.3f:end=%.3f,asetpts=PTS-STARTPTS", s.From, s.To)
			if s.Length() > 4*edgeFade {
				audio += fmt.Sprintf(",afade=t=in:st=0:d=%g,afade=t=out:st=%.3f:d=%g",
					edgeFade, s.Length()-edgeFade, edgeFade)
			}
			parts = append(parts, audio+fmt.Sprintf("[ta%d]", i))
			fmt.Fprintf(&chain, "[ta%d]", i)
		}
		parts = append(parts, fmt.Sprintf("%sconcat=n=%d:v=0:a=1[ac]", chain.String(), len(spans)))
		voice = "[ac]"
		mapA = "[ac]"
	}

	// Every take at the same loudness, with a limiter so a take raised a long
	// way cannot clip on its loudest syllable.
	if p.Level != 0 && !p.Silent {
		parts = append(parts, fmt.Sprintf(
			"%svolume=%.2fdB,alimiter=limit=0.89:level=false:latency=true[lv]", voice, p.Level))
		voice = "[lv]"
		mapA = "[lv]"
	}

	if len(p.Cues) > 0 {
		var labels strings.Builder
		for i, c := range p.Cues {
			ms := int(c.At*1000 + 0.5)
			parts = append(parts, fmt.Sprintf("[%d:a]adelay=%d|%d,volume=%g[c%d]", i+2, ms, ms, p.Gain, i+2))
			fmt.Fprintf(&labels, "[c%d]", i+2)
		}
		// normalize=0 keeps the narration at its recorded level instead of
		// ducking it whenever a quote overlaps. duration=first bounds the mix
		// to the microphone track, so a quote fired near the end cannot
		// stretch the piece past the words it belongs to.
		parts = append(parts, fmt.Sprintf(
			"%s%samix=inputs=%d:normalize=0:dropout_transition=0:duration=first[a]",
			voice, labels.String(), len(p.Cues)+1))
		mapA = "[a]"
	}

	// The sound padded past its end, so that -t, not the sound, decides where
	// the piece stops: both streams then end on the same frame.
	audioIn := mapA
	if !strings.HasPrefix(audioIn, "[") {
		audioIn = "[" + audioIn + "]"
	}
	parts = append(parts, audioIn+"apad[ap]")

	return Args{
		Inputs:  inputs,
		Filter:  strings.Join(parts, ";"),
		MapV:    "[v]",
		MapA:    "[ap]",
		Seconds: FrameLength(p.Kept()),
		Silent:  p.Silent,
	}
}

// FrameRate is the video's frames per second.
const FrameRate = 25

// FrameLength rounds a piece's length to whole frames, at least one.
//
// The pieces are joined end to end, and each one's picture can only end on a
// frame. A piece of 7.221 s of sound came out with 7.56 s of picture; over
// seventy pieces the picture fell five seconds behind the sound, and a figure
// appeared seconds after its caption was being read. A length in whole frames,
// with the sound padded or trimmed to exactly it, keeps the two together.
func FrameLength(seconds float64) float64 {
	frames := math.Round(seconds * FrameRate)
	return math.Max(1, frames) / FrameRate
}

// CommandLine renders the args as a full ffmpeg invocation.
func (a Args) CommandLine(output string) []string {
	cmd := []string{"ffmpeg", "-y", "-hide_banner", "-loglevel", "error", "-progress", "pipe:1", "-nostats"}
	for i, in := range a.Inputs {
		switch {
		// The page is a single image and has to be told to keep being one.
		case i == 0:
			cmd = append(cmd, "-loop", "1", "-framerate", "25")
		// Generated silence is a filter, not a file.
		case i == 1 && a.Silent:
			cmd = append(cmd, "-f", "lavfi")
		}
		cmd = append(cmd, "-i", in)
	}
	// -t, not -shortest. The still would loop for ever, and -shortest used to
	// end the piece at whichever stream ran out first — which was the sparse
	// captured video, clipping the last fraction of a second off every take's
	// speech. The length of a piece is the length of its sound.
	cmd = append(cmd,
		"-filter_complex", a.Filter,
		"-map", a.MapV, "-map", a.MapA,
		"-t", fmt.Sprintf("%.4f", a.Seconds),
		"-r", fmt.Sprint(FrameRate),
		"-c:v", "libx264", "-preset", "medium", "-crf", "20", "-pix_fmt", "yuv420p",
		// PCM, not AAC, until the pieces are joined: AAC comes in blocks of
		// 1024 samples, so every piece's sound would run a little past its
		// picture and the joins would drift. PCM ends on the sample.
		"-c:a", "pcm_s16le", "-ar", "48000", "-ac", "2",
		output,
	)
	return cmd
}
