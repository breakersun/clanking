package synth_test

import (
	"math"
	"testing"
	"time"

	"github.com/chrchr/clanking/internal/approx"
	"github.com/chrchr/clanking/synth"
)

const (
	testPeakDB     = -18.0
	pitchTolerance = 0.005
	levelTolerance = 1e-6
	sustain        = time.Hour
	testRoot       = 74
	concertA       = 69
)

func ms(n int) int { return n * synth.SampleRate / 1000 }

func pitchHz(samples []float32) float64 {
	var first, last, count int
	for i := 1; i < len(samples); i++ {
		if (samples[i-1] < 0) != (samples[i] < 0) {
			if count == 0 {
				first = i
			}
			last = i
			count++
		}
	}
	return float64(count-1) / 2 / (float64(last-first) / synth.SampleRate)
}

func peak(samples []float32) float64 {
	loudest := 0.0
	for _, x := range samples {
		loudest = max(loudest, math.Abs(float64(x)))
	}
	return loudest
}

func tonic(length time.Duration) []synth.Note {
	return []synth.Note{{From: 0, To: 0, Length: length, Decay: sustain, Timbre: synth.Sine}}
}

func TestAmplitudeOfMinusTwentyDecibels(t *testing.T) {
	if got := synth.Amplitude(-20); !approx.Equal(0.1, got, levelTolerance) {
		t.Errorf("Amplitude(-20) = %v, want 0.1", got)
	}
}

func TestMIDINoteInvertsHz(t *testing.T) {
	if got := synth.MIDINote(synth.Hz(testRoot)); !approx.Equal(testRoot, got, levelTolerance) {
		t.Errorf("MIDINote(Hz(%d)) = %v, want %d", testRoot, got, testRoot)
	}
}

func TestConcertAIs440Hz(t *testing.T) {
	if got := synth.Hz(concertA); got != 440 {
		t.Errorf("Hz(%d) = %v, want 440", concertA, got)
	}
}

func TestRenderStartsSilent(t *testing.T) {
	if got := synth.Render(tonic(50*time.Millisecond), testRoot, testPeakDB)[0]; got != 0 {
		t.Errorf("first sample = %v, want 0", got)
	}
}

func TestRenderEndsSilent(t *testing.T) {
	samples := synth.Render(tonic(50*time.Millisecond), testRoot, testPeakDB)
	if got := samples[len(samples)-1]; got != 0 {
		t.Errorf("last sample = %v, want 0", got)
	}
}

func TestRenderLengthCoversLatestNote(t *testing.T) {
	score := []synth.Note{
		{From: 0, To: 0, Length: 50 * time.Millisecond, Decay: sustain, Timbre: synth.Sine},
		{From: 0, To: 0, Onset: 30 * time.Millisecond, Length: 50 * time.Millisecond, Decay: sustain, Timbre: synth.Sine},
	}
	if got := len(synth.Render(score, testRoot, testPeakDB)); got != ms(80) {
		t.Errorf("len = %d, want %d", got, ms(80))
	}
}

func TestRenderPeaksAtRequestedLevel(t *testing.T) {
	got := peak(synth.Render(tonic(50*time.Millisecond), testRoot, testPeakDB))
	if !approx.Equal(synth.Amplitude(testPeakDB), got, levelTolerance) {
		t.Errorf("peak = %v, want %v", got, synth.Amplitude(testPeakDB))
	}
}

func TestRenderSteadyNoteSoundsAtRoot(t *testing.T) {
	if got := pitchHz(synth.Render(tonic(200*time.Millisecond), concertA, testPeakDB)); !approx.Equal(440, got, pitchTolerance) {
		t.Errorf("pitch = %v, want 440", got)
	}
}

func TestRenderGlideApproachesTarget(t *testing.T) {
	glide := []synth.Note{{From: 0, To: 7, Length: 250 * time.Millisecond, Decay: sustain, Timbre: synth.Sine}}
	samples := synth.Render(glide, testRoot, testPeakDB)
	want := synth.Hz(testRoot + 7*240.0/250.0)
	if got := pitchHz(samples[ms(235):ms(245)]); !approx.Equal(want, got, 2*pitchTolerance) {
		t.Errorf("pitch near end = %v, want %v", got, want)
	}
}
