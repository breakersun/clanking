package synth

import (
	"maps"
	"math"
	"slices"
	"testing"
)

var majorScale = []float64{0, 2, 4, 5, 7, 9, 11}

func inMajorScale(semitones float64) bool {
	return slices.Contains(majorScale, math.Mod(math.Mod(semitones, semitonesPerOctave)+semitonesPerOctave, semitonesPerOctave))
}

func scoredPitches() []float64 {
	pitches := slices.Clone(tickNotes)
	for _, s := range slices.Concat(slices.Collect(maps.Values(scores)), slices.Collect(maps.Values(themeScores))) {
		for _, n := range s.notes {
			pitches = append(pitches, n.From, n.To)
		}
	}
	return pitches
}

func TestEveryCueNoteIsInTheMajorScale(t *testing.T) {
	off := slices.DeleteFunc(scoredPitches(), inMajorScale)
	if len(off) != 0 {
		t.Errorf("off-scale pitches (semitones above tonic) = %v", off)
	}
}

func TestTonicIsTheRootPositionMajorTriad(t *testing.T) {
	var pitches []float64
	for _, n := range scores[Tonic].notes {
		pitches = append(pitches, n.From)
	}
	if want := []float64{0, 4, 7}; !slices.Equal(pitches, want) {
		t.Errorf("tonic pitches = %v, want %v", pitches, want)
	}
}
