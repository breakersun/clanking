package mixer_test

import (
	"math"
	"testing"

	"meknik/internal/approx"
	"meknik/mixer"
	"meknik/pan"
	"meknik/synth"
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
	hardLeft     = -1.0
	hardRight    = 1.0
)

func constant(value float32, frames int, tick bool) mixer.Sound {
	samples := make([]float32, frames)
	for i := range samples {
		samples[i] = value
	}
	cue := synth.Done
	if tick {
		cue = synth.Tick
	}
	return mixer.Sound{Samples: samples, Cue: cue, Pan: hardLeft}
}

func newBlock() []float32 { return make([]float32, pan.Channels*blockSize) }

func firstFrame(m *mixer.Mixer) float64 {
	block := newBlock()
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
	m.Fill(newBlock())
	if got := m.Voices(); got != 0 {
		t.Errorf("Voices = %d, want 0", got)
	}
}

func TestUnfinishedVoiceContinuesInNextBlock(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.Add(constant(quiet, blockSize+1, false))
	m.Fill(newBlock())
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

func TestGainShiftsLevelAboveCeiling(t *testing.T) {
	m := mixer.New(4*unity, tightCeiling)
	m.Add(constant(saturating, blockSize, false))
	want := 4 * tightCeiling * math.Tanh(saturating/tightCeiling)
	if got := firstFrame(m); !approx.Equal(want, got, tolerance) {
		t.Errorf("frame = %v, want %v", got, want)
	}
}

func TestSetGainChangesLevel(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.SetGain(0)
	m.Add(constant(quiet, blockSize, false))
	if got := firstFrame(m); got != 0 {
		t.Errorf("frame = %v, want 0", got)
	}
}

func TestAddAtDelaysFirstSample(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.AddAt(constant(loud, 1, false), 2)
	block := newBlock()
	m.Fill(block)
	if !approx.Equal(loud, float64(block[pan.Channels*2]), tolerance) {
		t.Errorf("frame 2 left = %v, want %v", block[pan.Channels*2], loud)
	}
}

func TestAddAtLeavesEarlierFramesSilent(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.AddAt(constant(loud, 1, false), 2)
	block := newBlock()
	m.Fill(block)
	if block[pan.Channels*1] != 0 {
		t.Errorf("frame 1 left = %v, want 0", block[pan.Channels*1])
	}
}

func TestDelayedVoiceContinuesInNextBlock(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.AddAt(constant(loud, blockSize, false), blockSize-1)
	m.Fill(newBlock())
	if got := firstFrame(m); !approx.Equal(loud, got, tolerance) {
		t.Errorf("next block first frame = %v, want %v", got, loud)
	}
}

func TestRightPannedSoundIsLouderOnRight(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.Add(mixer.Sound{Samples: []float32{loud}, Pan: pan.Width})
	block := newBlock()
	m.Fill(block)
	if block[1] <= block[0] {
		t.Errorf("left, right = %v, %v, want right louder", block[0], block[1])
	}
}

func TestHardRightSilencesLeft(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.Add(mixer.Sound{Samples: []float32{loud}, Pan: hardRight})
	block := newBlock()
	m.Fill(block)
	if math.Abs(float64(block[0])) > tolerance {
		t.Errorf("left = %v, want 0", block[0])
	}
}

func TestSecondFrameFollowsFirstPair(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.Add(mixer.Sound{Samples: []float32{quiet, loud}, Pan: hardLeft})
	block := newBlock()
	m.Fill(block)
	if !approx.Equal(loud, float64(block[pan.Channels]), tolerance) {
		t.Errorf("second frame left = %v, want %v", block[pan.Channels], loud)
	}
}
