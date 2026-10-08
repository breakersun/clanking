package synth_test

import (
	"math/rand/v2"
	"slices"
	"testing"

	"meknik/internal/approx"
	"meknik/synth"
)

const (
	tickLowHz  = 1800.0
	tickHighHz = 2600.0
)

func newRNG() *rand.Rand { return rand.New(rand.NewPCG(1, 2)) }

func sound(t *testing.T, c synth.Cue) []float32 {
	t.Helper()
	samples, ok := synth.Sound(c, newRNG())
	if !ok {
		t.Fatalf("Sound(%q) unknown", c)
	}
	return samples
}

func TestCueDurations(t *testing.T) {
	want := map[synth.Cue]int{
		synth.Tick: ms(12), synth.Spawn: ms(80), synth.Return: ms(80), synth.Done: ms(380),
		synth.Failed: ms(300), synth.Permission: ms(370), synth.Ask: ms(250), synth.Reply: ms(50), synth.Milestone: ms(335), synth.Noted: ms(120),
	}
	for c, frames := range want {
		t.Run(string(c), func(t *testing.T) {
			if got := len(sound(t, c)); got != frames {
				t.Errorf("len = %d, want %d", got, frames)
			}
		})
	}
}

func TestCuesStartSilent(t *testing.T) {
	for _, c := range synth.Cues {
		t.Run(string(c), func(t *testing.T) {
			if got := sound(t, c)[0]; got != 0 {
				t.Errorf("first sample = %v, want 0", got)
			}
		})
	}
}

func TestCuesEndSilent(t *testing.T) {
	for _, c := range synth.Cues {
		t.Run(string(c), func(t *testing.T) {
			samples := sound(t, c)
			if got := samples[len(samples)-1]; got != 0 {
				t.Errorf("last sample = %v, want 0", got)
			}
		})
	}
}

func TestCuePeakLevels(t *testing.T) {
	want := map[synth.Cue]float64{
		synth.Spawn: synth.CuePeakDB, synth.Return: synth.CuePeakDB, synth.Done: synth.CuePeakDB,
		synth.Failed: synth.CuePeakDB, synth.Permission: synth.CuePeakDB, synth.Ask: synth.CuePeakDB,
		synth.Reply: synth.ReplyPeakDB, synth.Milestone: synth.CuePeakDB, synth.Noted: synth.ReplyPeakDB,
	}
	for c, db := range want {
		t.Run(string(c), func(t *testing.T) {
			if got := peak(sound(t, c)); !approx.Equal(synth.Amplitude(db), got, levelTolerance) {
				t.Errorf("peak = %v, want %v", got, synth.Amplitude(db))
			}
		})
	}
}

func TestReplySitsBetweenTicksAndCues(t *testing.T) {
	if got := synth.ReplyPeakDB; got != -33 {
		t.Errorf("ReplyPeakDB = %v, want -33", got)
	}
}

func TestTickStaysUnderJitteredLevel(t *testing.T) {
	if got := peak(sound(t, synth.Tick)); got > synth.Amplitude(synth.TickPeakDB+synth.TickJitterDB) {
		t.Errorf("peak = %v, want <= %v", got, synth.Amplitude(synth.TickPeakDB+synth.TickJitterDB))
	}
}

func TestTickPitchInRange(t *testing.T) {
	if got := pitchHz(sound(t, synth.Tick)); got < tickLowHz*(1-pitchTolerance) || got > tickHighHz*(1+pitchTolerance) {
		t.Errorf("pitch = %v, want within [%v, %v]", got, tickLowHz, tickHighHz)
	}
}

func TestCueFinalPitch(t *testing.T) {
	cases := []struct {
		cue       synth.Cue
		fromMs    int
		toMs      int
		semitones float64
	}{
		{synth.Spawn, 45, 75, 19},
		{synth.Return, 45, 75, 12},
		{synth.Done, 300, 370, 0},
		{synth.Permission, 200, 360, 11},
		{synth.Reply, 28, 48, 24},
		{synth.Milestone, 145, 300, 12},
		{synth.Noted, 10, 60, 12},
	}
	for _, tc := range cases {
		t.Run(string(tc.cue), func(t *testing.T) {
			got := pitchHz(sound(t, tc.cue)[ms(tc.fromMs):ms(tc.toMs)])
			if !approx.Equal(synth.Hz(tc.semitones), got, pitchTolerance) {
				t.Errorf("pitch = %v, want %v", got, synth.Hz(tc.semitones))
			}
		})
	}
}

func TestUnknownCueIsRejected(t *testing.T) {
	if _, ok := synth.Sound("bogus", newRNG()); ok {
		t.Error("Sound(bogus) ok = true, want false")
	}
}

func TestDoneSitsWellBelowFullScale(t *testing.T) {
	if got := peak(sound(t, synth.Done)); !approx.Equal(synth.Amplitude(-30), got, levelTolerance) {
		t.Errorf("peak = %v, want %v", got, synth.Amplitude(-30))
	}
}

func TestTickSitsBelowCues(t *testing.T) {
	if got := synth.CuePeakDB - synth.TickPeakDB; got != 6 {
		t.Errorf("cue minus tick = %v dB, want 6", got)
	}
}

func TestCuesListsEveryCueEndingOnDone(t *testing.T) {
	want := []synth.Cue{synth.Tick, synth.Spawn, synth.Return, synth.Failed, synth.Permission, synth.Ask, synth.Reply, synth.Milestone, synth.Noted, synth.Done}
	if !slices.Equal(synth.Cues, want) {
		t.Errorf("Cues = %v, want %v", synth.Cues, want)
	}
}

func TestEveryListedCueHasASound(t *testing.T) {
	rng := newRNG()
	for _, c := range synth.Cues {
		t.Run(string(c), func(t *testing.T) {
			if _, ok := synth.Sound(c, rng); !ok {
				t.Errorf("Sound(%q) unknown", c)
			}
		})
	}
}
