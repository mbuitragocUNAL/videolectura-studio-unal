package render

import (
	"math"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

func momentaryLoudness(t *testing.T, path string, from, to float64) float64 {
	t.Helper()
	out, err := exec.Command("ffmpeg", "-hide_banner", "-nostats", "-ss", strconv.FormatFloat(from, 'f', 2, 64),
		"-to", strconv.FormatFloat(to, 'f', 2, 64), "-i", path, "-vn", "-af", "ebur128", "-f", "null", "-").CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	m := regexp.MustCompile(`I:\s+(-?[\d.]+) LUFS`).FindAllStringSubmatch(string(out), -1)
	v, _ := strconv.ParseFloat(m[len(m)-1][1], 64)
	return v
}

// The track sits under the voice by the level asked for, keeps the video,
// and runs exactly as long as the reading.
func TestAddMusicSitsUnderTheVoice(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	dir := t.TempDir()
	voice := filepath.Join(dir, "final.mp4")
	music := filepath.Join(dir, "pad.wav")
	// Twelve seconds: «speech» (a 300 Hz tone) from 2 to 10 s, silence around it.
	run := func(args ...string) {
		if out, err := exec.Command("ffmpeg", append([]string{"-v", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	run("-f", "lavfi", "-i", "color=c=white:s=320x180:d=12", "-f", "lavfi",
		"-i", "sine=frequency=300:duration=12", "-af", "volume=enable='between(t,2,10)':volume=1,volume=enable='not(between(t,2,10))':volume=0",
		"-shortest", "-c:v", "libx264", "-c:a", "aac", voice)
	run("-f", "lavfi", "-i", "sine=frequency=110:duration=5", "-ac", "2", music) // shorter: must loop

	before := momentaryLoudness(t, voice, 3, 9)
	if err := addMusic(&Job{}, voice, Music{File: music, Below: 20, FadeIn: 1, FadeOut: 1}, before); err != nil {
		t.Fatal(err)
	}
	if d, _ := ProbeDuration(voice); d < 11.8 || d > 12.2 {
		t.Errorf("duration %.2f, want 12", d)
	}
	// In the silence after the voice, before the fade: the bed alone.
	bed := momentaryLoudness(t, voice, 10.2, 10.9)
	if below := before - bed; below < 17 || below > 23 {
		t.Errorf("the bed is %.1f dB under the voice, want about 20", below)
	}
}

// Ducking lowers the bed while there is voice, and lets it back in the pauses.
func TestAddMusicDucksUnderTheVoice(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		if out, err := exec.Command("ffmpeg", append([]string{"-v", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	music := filepath.Join(dir, "pad.wav")
	run("-f", "lavfi", "-i", "sine=frequency=110:duration=12", "-ac", "2", music)
	bedUnderVoice := func(duck bool) float64 {
		voice := filepath.Join(dir, "final.mp4")
		run("-f", "lavfi", "-i", "color=c=white:s=320x180:d=12", "-f", "lavfi",
			"-i", "sine=frequency=3000:duration=12", "-af", "volume=enable='not(between(t,2,10))':volume=0",
			"-shortest", "-c:v", "libx264", "-c:a", "aac", voice)
		if err := addMusic(&Job{}, voice, Music{File: music, Below: 20, Duck: duck, FadeIn: 1, FadeOut: 1}, -16); err != nil {
			t.Fatal(err)
		}
		// Only the bed's 110 Hz: low-pass away the 3 kHz «voice».
		out, err := exec.Command("ffmpeg", "-hide_banner", "-nostats", "-ss", "4", "-to", "9", "-i", voice,
			"-af", "lowpass=f=300,lowpass=f=300,ebur128", "-f", "null", "-").CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		m := regexp.MustCompile(`I:\s+(-?[\d.]+) LUFS`).FindAllStringSubmatch(string(out), -1)
		v, _ := strconv.ParseFloat(m[len(m)-1][1], 64)
		return v
	}
	plain, ducked := bedUnderVoice(false), bedUnderVoice(true)
	if plain-ducked < 3 {
		t.Errorf("ducking took the bed from %.1f to %.1f LUFS under the voice, want at least 3 dB down", plain, ducked)
	}
}

// A song ends in silence; repeated, the bed must not drop out at each seam.
func TestAddMusicHasNoGapWhereTheTrackRepeats(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		if out, err := exec.Command("ffmpeg", append([]string{"-v", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	voice := filepath.Join(dir, "final.mp4")
	music := filepath.Join(dir, "song.wav")
	run("-f", "lavfi", "-i", "color=c=white:s=320x180:d=40", "-f", "lavfi", "-i", "anullsrc=r=48000:cl=mono",
		"-t", "40", "-c:v", "libx264", "-c:a", "aac", voice)
	// 16 s of tone, then 2 s of silence: a song with an ending.
	run("-f", "lavfi", "-i", "sine=frequency=220:duration=16", "-af", "apad=pad_dur=2", "-ac", "2", music)
	if err := addMusic(&Job{}, voice, Music{File: music, Below: 20, FadeIn: 1, FadeOut: 1}, -16); err != nil {
		t.Fatal(err)
	}
	// Second by second through the seams (about 14–18 s and 28–32 s), the
	// bed never falls more than a few dB.
	out, err := exec.Command("ffmpeg", "-v", "error", "-ss", "3", "-to", "36", "-i", voice, "-vn", "-ac", "1",
		"-ar", "8000", "-f", "s16le", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	lowest, highest := 0.0, -200.0
	for i := 0; i+16000 <= len(out); i += 16000 { // one second of 16-bit samples
		var sum float64
		for j := i; j < i+16000; j += 2 {
			v := float64(int16(uint16(out[j])|uint16(out[j+1])<<8)) / 32768
			sum += v * v
		}
		db := 10 * math.Log10(sum/8000+1e-12)
		lowest, highest = math.Min(lowest, db), math.Max(highest, db)
	}
	if highest-lowest > 6 {
		t.Errorf("the bed swings %.1f dB (%.1f to %.1f): it drops out where the track repeats", highest-lowest, lowest, highest)
	}
}
