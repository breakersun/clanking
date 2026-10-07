package synth

import (
	"math/rand/v2"
	"time"
)

// Cue names one kind of activity sound.
type Cue string

// The cues the daemon can play.
const (
	Tick       Cue = "tick"
	Spawn      Cue = "spawn"
	Return     Cue = "return"
	Done       Cue = "done"
	Failed     Cue = "failed"
	Permission Cue = "permission"
	Ask        Cue = "ask"
)

// Peak levels in dBFS, with the random level spread applied to ticks.
const (
	TickPeakDB   = -36.0
	CuePeakDB    = -30.0
	TickJitterDB = 3.0
)

const (
	tickLowHz  = 1800.0
	tickHighHz = 2600.0
	tickLength = 12 * time.Millisecond
	tickDecay  = 3 * time.Millisecond
	sustain    = time.Hour
	bloop      = 40 * time.Millisecond
	cadenceGap = 90 * time.Millisecond
	ring       = 200 * time.Millisecond
	ringDecay  = 80 * time.Millisecond
	buzzNote   = 150 * time.Millisecond
	knock      = 60 * time.Millisecond
	hang       = 250 * time.Millisecond
	glide      = 250 * time.Millisecond
)

var scores = map[Cue][]Note{
	Spawn:  {held(12, 0, bloop), held(19, bloop, bloop)},
	Return: {held(19, 0, bloop), held(12, bloop, bloop)},
	Done:   {rung(7, 0), rung(4, cadenceGap), rung(0, 2*cadenceGap)},
	Failed: {
		{From: -6, To: -6, Length: buzzNote, Decay: sustain, Timbre: Buzzy},
		{From: -12, To: -12, Onset: buzzNote, Length: buzzNote, Decay: sustain, Timbre: Buzzy},
	},
	Permission: {held(5, 0, knock), held(5, knock, knock), held(11, 2*knock, hang)},
	Ask:        {{From: 0, To: 7, Length: glide, Decay: sustain, Timbre: Sine}},
}

// Sound renders a cue, reporting false for an unknown cue.
func Sound(c Cue, rng *rand.Rand) ([]float32, bool) {
	if c == Tick {
		return tick(rng), true
	}
	score, ok := scores[c]
	if !ok {
		return nil, false
	}
	return Render(score, CuePeakDB), true
}

func tick(rng *rand.Rand) []float32 {
	pitch := Semitones(tickLowHz + rng.Float64()*(tickHighHz-tickLowHz))
	jitter := (2*rng.Float64() - 1) * TickJitterDB
	return Render([]Note{{From: pitch, To: pitch, Length: tickLength, Decay: tickDecay, Timbre: Sine}}, TickPeakDB+jitter)
}

func held(semitones float64, onset, length time.Duration) Note {
	return Note{From: semitones, To: semitones, Onset: onset, Length: length, Decay: sustain, Timbre: Sine}
}

func rung(semitones float64, onset time.Duration) Note {
	return Note{From: semitones, To: semitones, Onset: onset, Length: ring, Decay: ringDecay, Timbre: Sine}
}
