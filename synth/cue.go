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
	Reply      Cue = "reply"
	Milestone  Cue = "milestone"
	Noted      Cue = "noted"
)

// tickNotes are the D major pentatonic notes A6 to F#7, in semitones above the tonic.
var tickNotes = []float64{19, 21, 24, 26, 28}

// Cues lists every cue in self-test order, ending on Done.
var Cues = []Cue{Tick, Spawn, Return, Failed, Permission, Ask, Reply, Milestone, Noted, Done}

// Peak levels in dBFS, with the random level spread applied to ticks.
const (
	TickPeakDB   = -36.0
	CuePeakDB    = -30.0
	ReplyPeakDB  = -33.0
	TickJitterDB = 3.0
)

const (
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
	bellGap    = 110 * time.Millisecond
	blip       = 25 * time.Millisecond
	climb      = 45 * time.Millisecond
	plink      = 120 * time.Millisecond
	plinkDecay = 40 * time.Millisecond
)

type score struct {
	peakDB float64
	notes  []Note
}

var scores = map[Cue]score{
	Spawn:  {CuePeakDB, []Note{held(12, 0, bloop), held(19, bloop, bloop)}},
	Return: {CuePeakDB, []Note{held(19, 0, bloop), held(12, bloop, bloop)}},
	Done:   {CuePeakDB, []Note{rung(7, 0), rung(4, cadenceGap), rung(0, 2*cadenceGap)}},
	Failed: {CuePeakDB, []Note{
		{From: -6, To: -6, Length: buzzNote, Decay: sustain, Timbre: Buzzy},
		{From: -12, To: -12, Onset: buzzNote, Length: buzzNote, Decay: sustain, Timbre: Buzzy},
	}},
	Permission: {CuePeakDB, []Note{held(5, 0, knock), held(5, knock, knock), held(11, 2*knock, hang)}},
	Ask:        {CuePeakDB, []Note{rung(0, 0), rung(5, bellGap)}},
	Reply:      {ReplyPeakDB, []Note{held(19, 0, blip), held(24, blip, blip)}},
	Milestone:  {CuePeakDB, []Note{held(0, 0, climb), held(4, climb, climb), held(7, 2*climb, climb), rung(12, 3*climb)}},
	Noted:      {ReplyPeakDB, []Note{{From: 12, To: 12, Length: plink, Decay: plinkDecay, Timbre: Sine}}},
}

// Sound renders a cue, reporting false for an unknown cue.
func Sound(c Cue, rng *rand.Rand) ([]float32, bool) {
	if c == Tick {
		return tick(rng), true
	}
	s, ok := scores[c]
	if !ok {
		return nil, false
	}
	return Render(s.notes, s.peakDB), true
}

func tick(rng *rand.Rand) []float32 {
	pitch := tickNotes[rng.IntN(len(tickNotes))]
	jitter := (2*rng.Float64() - 1) * TickJitterDB
	return Render([]Note{{From: pitch, To: pitch, Length: tickLength, Decay: tickDecay, Timbre: Sine}}, TickPeakDB+jitter)
}

func held(semitones float64, onset, length time.Duration) Note {
	return Note{From: semitones, To: semitones, Onset: onset, Length: length, Decay: sustain, Timbre: Sine}
}

func rung(semitones float64, onset time.Duration) Note {
	return Note{From: semitones, To: semitones, Onset: onset, Length: ring, Decay: ringDecay, Timbre: Sine}
}
