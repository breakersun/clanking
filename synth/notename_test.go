package synth_test

import (
	"testing"

	"github.com/chrchr/clanking/synth"
)

const (
	middleC     = 60
	highestMIDI = 127
)

func TestNoteNameOfDefaultRoot(t *testing.T) {
	if got := synth.NoteName(testRoot); got != "D5" {
		t.Errorf("NoteName(%d) = %q, want D5", testRoot, got)
	}
}

func TestNoteNameUsesSharps(t *testing.T) {
	if got := synth.NoteName(testRoot + 1); got != "D♯5" {
		t.Errorf("NoteName(%d) = %q, want D♯5", testRoot+1, got)
	}
}

func TestNoteNameOfMiddleC(t *testing.T) {
	if got := synth.NoteName(middleC); got != "C4" {
		t.Errorf("NoteName(%d) = %q, want C4", middleC, got)
	}
}

func TestParseNoteNameInvertsNoteName(t *testing.T) {
	var wrong []int
	for n := range highestMIDI + 1 {
		if got, ok := synth.ParseNoteName(synth.NoteName(n)); !ok || got != n {
			wrong = append(wrong, n)
		}
	}
	if len(wrong) != 0 {
		t.Errorf("notes that do not round-trip = %v", wrong)
	}
}

func TestParseNoteNameRejectsUnknownName(t *testing.T) {
	if _, ok := synth.ParseNoteName("H5"); ok {
		t.Error("ParseNoteName(H5) ok = true, want false")
	}
}
