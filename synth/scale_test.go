package synth

import (
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
	for _, s := range scores {
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
