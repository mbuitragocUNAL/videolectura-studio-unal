package render

import (
	"fmt"
	"math"
	"os"
	"strings"
)

// Music is a track laid under the whole reading (lineamiento 2.j: «música de
// fondo con un volumen apropiado para que no nuble la voz de quien lee»).
type Music struct {
	File string
	// Below is how many dB under the narration the track sits, by integrated
	// loudness. Fourteen is a bed you hear behind the words; twenty, tried
	// first, measured right and was inaudible on a laptop's speakers.
	Below float64
	// Duck lowers the track a little more while the reader is speaking and
	// lets it back up in the pauses, the way a radio host's bed behaves.
	Duck bool
	// FadeIn and FadeOut, in seconds, at the very start and end.
	FadeIn, FadeOut float64
}

// crossfade is how long one repetition of the track overlaps the next. A song
// ends (a final chord, a second of silence) and starts again; laid end to end
// that is a restart every few minutes under the reading. Overlapped, the end
// rings out while the start comes in, and the bed never stops.
const crossfade = 4.0

// musicArgs lays music under the narration in voice. The track repeats, each
// time crossfaded into the next, for as long as the video runs; the video
// stream is copied untouched.
func musicArgs(voice, output string, m Music, narration, musicLUFS, duration, trackLen float64) []string {
	gain := (narration - m.Below) - musicLUFS
	if math.IsInf(musicLUFS, 0) || math.IsNaN(musicLUFS) {
		gain = 0
	}
	gain = math.Max(-60, math.Min(20, gain))
	fadeOut := math.Max(0, duration-m.FadeOut)

	// A short track gets a short overlap: never more than a quarter of it,
	// or the fades of neighbouring repetitions would overlap each other.
	xf := math.Min(crossfade, trackLen/4)
	copies := 1
	if trackLen > 0 && duration > trackLen {
		copies = int(math.Ceil((duration-xf)/(trackLen-xf))) + 1
	}
	copies = max(1, min(copies, 60))
	var loop strings.Builder
	for i := 1; i <= copies; i++ {
		fmt.Fprintf(&loop, "[%d:a]aformat=sample_rates=48000:channel_layouts=stereo[m%d];", i, i)
	}
	last := "m1"
	for i := 2; i <= copies; i++ {
		fmt.Fprintf(&loop, "[%s][m%d]acrossfade=d=%.2f:c1=tri:c2=tri[x%d];", last, i, xf, i)
		last = fmt.Sprintf("x%d", i)
	}
	bed := loop.String() + fmt.Sprintf("[%s]"+
		"atrim=0:%.3f,asetpts=PTS-STARTPTS,volume=%.2fdB,"+
		"afade=t=in:st=0:d=%.2f,afade=t=out:st=%.3f:d=%.2f[bed]",
		last, duration, gain, m.FadeIn, fadeOut, m.FadeOut)
	voiceIn := "[0:a]aformat=sample_rates=48000:channel_layouts=stereo"
	var graph string
	if m.Duck {
		// The voice keys a gentle compressor on the bed: about 3 dB down
		// while there are words, released over half a second in a pause.
		graph = bed + ";" + voiceIn + ",asplit[v][key];" +
			"[bed][key]sidechaincompress=threshold=0.02:ratio=2:attack=80:release=600[ducked];" +
			"[v][ducked]amix=inputs=2:normalize=0:duration=first,alimiter=limit=0.95:level=false[a]"
	} else {
		graph = bed + ";" + voiceIn + "[v];" +
			"[v][bed]amix=inputs=2:normalize=0:duration=first,alimiter=limit=0.95:level=false[a]"
	}
	args := []string{"ffmpeg", "-y", "-hide_banner", "-loglevel", "error", "-i", voice}
	for range copies {
		args = append(args, "-i", m.File)
	}
	return append(args,
		"-filter_complex", graph,
		"-map", "0:v", "-map", "[a]", "-c:v", "copy", "-c:a", "aac", "-b:a", "192k",
		"-movflags", "+faststart", output)
}

// addMusic replaces output with itself plus the music.
func addMusic(job *Job, output string, m Music, narration float64) error {
	if narration == 0 {
		narration = -16
	}
	musicLUFS, err := Loudness(m.File)
	if err != nil {
		return fmt.Errorf("measuring the music: %w", err)
	}
	duration, err := ProbeDuration(output)
	if err != nil {
		return err
	}
	trackLen, err := ProbeDuration(m.File)
	if err != nil {
		return fmt.Errorf("reading the music's length: %w", err)
	}
	voice := output + ".voz.mp4"
	if err := os.Rename(output, voice); err != nil {
		return err
	}
	if err := runFFmpeg(job, musicArgs(voice, output, m, narration, musicLUFS, duration, trackLen), nil); err != nil {
		os.Rename(voice, output)
		return err
	}
	return os.Remove(voice)
}
