// Package grid quantizes sound starts to a rhythmic grid with a short queue.
package grid

import (
	"meknik/mixer"
	"meknik/synth"
)

const (
	beatsPerMinute    = 120
	secondsPerMinute  = 60
	beatsPerBar       = 4
	framesPerBeat     = synth.SampleRate * secondsPerMinute / beatsPerMinute
	sixteenth         = framesPerBeat / 4
	thirtySecond      = framesPerBeat / 8
	quietFrames       = beatsPerBar * framesPerBeat
	queueLimit        = 4
	dropsBeforeFaster = 2
)

// Start is a sound due to begin offset frames into the current block.
type Start struct {
	Sound  mixer.Sound
	Offset int
}

// Scheduler queues sounds and starts at most one per grid line.
type Scheduler struct {
	pending     []mixer.Sound
	slot        int
	clock       int
	free        int
	drops       int
	lastArrival int
}

// New returns a scheduler on a 16th-note grid at frame 0.
func New() *Scheduler { return &Scheduler{slot: sixteenth} }

// Offer queues a sound, reporting false when the queue is full and the sound is dropped.
func (s *Scheduler) Offer(snd mixer.Sound) bool {
	s.lastArrival = s.clock
	if len(s.pending) == queueLimit {
		s.drops++
		if s.drops > dropsBeforeFaster {
			s.slot = thirtySecond
		}
		return false
	}
	s.drops = 0
	s.pending = append(s.pending, snd)
	return true
}

// Due returns the sounds that start within the next frames frames and advances the clock.
func (s *Scheduler) Due(frames int) []Start {
	if s.slot == thirtySecond && s.clock-s.lastArrival >= quietFrames {
		s.slot = sixteenth
	}
	var due []Start
	for len(s.pending) > 0 {
		line := s.nextLine()
		if line >= s.clock+frames {
			break
		}
		due = append(due, Start{Sound: s.pending[0], Offset: line - s.clock})
		s.pending = s.pending[1:]
		s.free = line + s.slot
	}
	s.clock += frames
	return due
}

func (s *Scheduler) nextLine() int {
	from := max(s.clock, s.free)
	return (from + s.slot - 1) / s.slot * s.slot
}
