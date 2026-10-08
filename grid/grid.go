// Package grid quantizes sound starts to a rhythmic grid with a short queue.
package grid

import (
	"fmt"
	"github.com/chrchr/clanking/mixer"
	"github.com/chrchr/clanking/synth"
	"sync/atomic"
)

const (
	secondsPerMinute  = 60
	beatsPerBar       = 4
	sixteenths        = 16
	thirtySeconds     = 32
	stateFormat       = "in %s, %d queued, %d dropped"
	queueLimit        = 4
	dropsBeforeFaster = 2
)

var divisionNames = map[int]string{sixteenths: "16ths", thirtySeconds: "32nds"}

// State is a snapshot of the grid for status reports.
type State struct {
	Division int
	Queued   int
	Dropped  int
}

// String reports the division and queue counts.
func (st State) String() string {
	return fmt.Sprintf(stateFormat, divisionNames[st.Division], st.Queued, st.Dropped)
}

// Start is a sound due to begin offset frames into the current block.
type Start struct {
	Sound  mixer.Sound
	Offset int
}

// Scheduler queues sounds and starts at most one per grid line.
type Scheduler struct {
	pending     []mixer.Sound
	division    int
	whole       int
	clock       int
	free        int
	drops       int
	dropped     int
	lastArrival int
	state       atomic.Pointer[State]
}

// New returns a scheduler on a 16th-note grid at frame 0, at bpm beats per minute.
func New(bpm int) *Scheduler {
	s := &Scheduler{division: sixteenths}
	s.SetTempo(bpm)
	return s
}

// SetTempo sets the grid to bpm beats per minute.
func (s *Scheduler) SetTempo(bpm int) {
	s.whole = beatsPerBar * synth.SampleRate * secondsPerMinute / bpm
	s.publish()
}

func (s *Scheduler) slot() int { return s.whole / s.division }

// State returns the latest snapshot; it is safe to call from any goroutine.
func (s *Scheduler) State() State { return *s.state.Load() }

func (s *Scheduler) publish() {
	s.state.Store(&State{Division: s.division, Queued: len(s.pending), Dropped: s.dropped})
}

// Offer queues a sound, reporting false when the queue is full and the sound is dropped.
func (s *Scheduler) Offer(snd mixer.Sound) bool {
	defer s.publish()
	s.lastArrival = s.clock
	if len(s.pending) == queueLimit {
		s.drops++
		s.dropped++
		if s.drops > dropsBeforeFaster {
			s.division = thirtySeconds
		}
		return false
	}
	s.drops = 0
	s.pending = append(s.pending, snd)
	return true
}

// Due returns the sounds that start within the next frames frames and advances the clock.
func (s *Scheduler) Due(frames int) []Start {
	defer s.publish()
	if s.division == thirtySeconds && s.clock-s.lastArrival >= s.whole {
		s.division = sixteenths
	}
	var due []Start
	for len(s.pending) > 0 {
		line := s.nextLine()
		if line >= s.clock+frames {
			break
		}
		due = append(due, Start{Sound: s.pending[0], Offset: line - s.clock})
		s.pending = s.pending[1:]
		s.free = line + s.slot()
	}
	s.clock += frames
	return due
}

func (s *Scheduler) nextLine() int {
	slot := s.slot()
	from := max(s.clock, s.free)
	return (from + slot - 1) / slot * slot
}
