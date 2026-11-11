package render

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os/exec"
)

// LevelStep is the frame SpeechLevels measures in.
const LevelStep = 0.02

// SpeechLevels decodes a take's sound and returns its loudness in dB every
// LevelStep seconds. It reads the file itself rather than the levels the
// browser sampled while recording: those are coarser, and they were taken
// before the encoder, which is not what the render cuts.
func SpeechLevels(path string) ([]float64, error) {
	const rate = 16000
	out, err := exec.Command("ffmpeg", "-v", "error", "-i", path, "-vn", "-ac", "1",
		"-ar", fmt.Sprint(rate), "-f", "s16le", "-").Output()
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", path, err)
	}
	samples := make([]int16, len(out)/2)
	if err := binary.Read(bytes.NewReader(out[:len(samples)*2]), binary.LittleEndian, samples); err != nil {
		return nil, err
	}
	frame := int(rate * LevelStep)
	db := make([]float64, 0, len(samples)/frame)
	for i := 0; i+frame <= len(samples); i += frame {
		var sum float64
		for _, s := range samples[i : i+frame] {
			v := float64(s) / 32768
			sum += v * v
		}
		db = append(db, 10*math.Log10(sum/float64(frame)+1e-12))
	}
	return db, nil
}
