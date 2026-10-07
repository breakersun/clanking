package mixer_test

import (
	"testing"

	"meknik/internal/approx"
	"meknik/mixer"
)

const (
	unity        = 1.0
	wideCeiling  = 100.0
	tightCeiling = 0.25
	quiet        = 0.001
	saturating   = 0.1
	loud         = 0.5
	tolerance    = 1e-3
	blockSize    = 4
)

func constant(value float32, frames int, tick bool) mixer.Sound {
	samples := make([]float32, frames)
	for i := range samples {
		samples[i] = value
	}
	return mixer.Sound{Samples: samples, Tick: tick}
}

func firstFrame(m *mixer.Mixer) float64 {
	block := make([]float32, blockSize)
	m.Fill(block)
	return float64(block[0])
}

func fullPool(tick bool) *mixer.Mixer {
	m := mixer.New(unity, wideCeiling)
	for range mixer.MaxVoices {
		m.Add(constant(quiet, blockSize, tick))
	}
	return m
}

func TestFillSumsVoices(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.Add(constant(quiet, blockSize, true))
	m.Add(constant(quiet, blockSize, false))
	if got := firstFrame(m); !approx.Equal(2*quiet, got, tolerance) {
		t.Errorf("frame = %v, want %v", got, 2*quiet)
	}
}

func TestFillAppliesGain(t *testing.T) {
	m := mixer.New(2*unity, wideCeiling)
	m.Add(constant(quiet, blockSize, false))
	if got := firstFrame(m); !approx.Equal(2*quiet, got, tolerance) {
		t.Errorf("frame = %v, want %v", got, 2*quiet)
	}
}

func TestFillHoldsBelowCeiling(t *testing.T) {
	m := mixer.New(unity, tightCeiling)
	for range mixer.MaxVoices {
		m.Add(constant(saturating, blockSize, false))
	}
	if got := firstFrame(m); got >= tightCeiling {
		t.Errorf("frame = %v, want < %v", got, tightCeiling)
	}
}

func TestFinishedVoiceIsRetired(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.Add(constant(quiet, blockSize-1, false))
	m.Fill(make([]float32, blockSize))
	if got := m.Voices(); got != 0 {
		t.Errorf("Voices = %d, want 0", got)
	}
}

func TestUnfinishedVoiceContinuesInNextBlock(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.Add(constant(quiet, blockSize+1, false))
	m.Fill(make([]float32, blockSize))
	if got := firstFrame(m); !approx.Equal(quiet, got, tolerance) {
		t.Errorf("frame = %v, want %v", got, quiet)
	}
}

func TestFullPoolDropsTick(t *testing.T) {
	m := fullPool(true)
	m.Add(constant(loud, blockSize, true))
	if got := firstFrame(m); !approx.Equal(mixer.MaxVoices*quiet, got, tolerance) {
		t.Errorf("frame = %v, want %v", got, mixer.MaxVoices*quiet)
	}
}

func TestFullPoolCueReplacesTick(t *testing.T) {
	m := fullPool(true)
	m.Add(constant(loud, blockSize, false))
	want := (mixer.MaxVoices-1)*quiet + loud
	if got := firstFrame(m); !approx.Equal(want, got, tolerance) {
		t.Errorf("frame = %v, want %v", got, want)
	}
}

func TestFullPoolOfCuesDropsCue(t *testing.T) {
	m := fullPool(false)
	m.Add(constant(loud, blockSize, false))
	if got := firstFrame(m); !approx.Equal(mixer.MaxVoices*quiet, got, tolerance) {
		t.Errorf("frame = %v, want %v", got, mixer.MaxVoices*quiet)
	}
}
