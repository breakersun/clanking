package grid_test

import (
	"testing"

	"meknik/grid"
	"meknik/mixer"
	"meknik/synth"
)

const (
	block        = 256
	sixteenth    = 6000
	thirtySecond = 3000
	bar          = 96000
	queueLimit   = 4
)

func sound() mixer.Sound { return mixer.Sound{Cue: synth.Done} }

func offers(s *grid.Scheduler, n int) {
	for range n {
		s.Offer(sound())
	}
}

func starts(s *grid.Scheduler, frames int) []int {
	var at []int
	for clock := 0; clock < frames; clock += block {
		for _, st := range s.Due(block) {
			at = append(at, clock+st.Offset)
		}
	}
	return at
}

func TestFirstSoundStartsAtFrameZero(t *testing.T) {
	s := grid.New()
	offers(s, 1)
	if got := s.Due(block); len(got) != 1 || got[0].Offset != 0 {
		t.Errorf("Due = %+v, want one start at offset 0", got)
	}
}

func TestSecondSoundStartsOneSixteenthLater(t *testing.T) {
	s := grid.New()
	offers(s, 2)
	if got := starts(s, 2*sixteenth); len(got) != 2 || got[1] != sixteenth {
		t.Errorf("starts = %v, want [0 %d]", got, sixteenth)
	}
}

func TestSoundBetweenLinesWaitsForNextLine(t *testing.T) {
	s := grid.New()
	s.Due(block)
	offers(s, 1)
	if got := starts(s, sixteenth); len(got) != 1 || got[0] != sixteenth-block {
		t.Errorf("starts = %v, want [%d] (relative to frame %d)", got, sixteenth-block, block)
	}
}

func TestFifthWaitingSoundIsDropped(t *testing.T) {
	s := grid.New()
	offers(s, queueLimit)
	if s.Offer(sound()) {
		t.Error("Offer accepted a fifth waiting sound")
	}
}

func TestAcceptanceResetsTheDropStreak(t *testing.T) {
	s := grid.New()
	offers(s, queueLimit+2)
	s.Due(block)
	offers(s, 2)
	if got := starts(s, 5*sixteenth); len(got) != queueLimit || got[1]-got[0] != sixteenth {
		t.Errorf("starts = %v, want 16th spacing", got)
	}
}

func TestThirdDropInARowSwitchesToThirtySeconds(t *testing.T) {
	s := grid.New()
	offers(s, queueLimit+3)
	if got := starts(s, 2*thirtySecond); len(got) < 2 || got[1] != thirtySecond {
		t.Errorf("starts = %v, want [0 %d]", got, thirtySecond)
	}
}

func TestQuietBarReturnsToSixteenths(t *testing.T) {
	s := grid.New()
	offers(s, queueLimit+3)
	starts(s, bar+queueLimit*thirtySecond)
	offers(s, 2)
	if got := starts(s, 2*sixteenth); len(got) != 2 || got[1]-got[0] != sixteenth {
		t.Errorf("starts = %v, want 16th spacing", got)
	}
}

func TestSoundDueAfterTheBlockIsNotReturned(t *testing.T) {
	s := grid.New()
	offers(s, 2)
	s.Due(block)
	if got := s.Due(block); len(got) != 0 {
		t.Errorf("Due = %+v, want nothing before the next line", got)
	}
}
