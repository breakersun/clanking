// Package synth renders Claude Code activity cues as audio sample buffers.
package synth

import (
	"math"
	"time"
)

// SampleRate is the output rate in frames per second.
const SampleRate = 48000

const (
	concertANote       = 69
	concertAHz         = 440.0
	semitonesPerOctave = 12
	decibelsPerDecade  = 20
	attack             = 3 * time.Millisecond
	release            = 5 * time.Millisecond
)

// Partial is one harmonic of a timbre with its relative amplitude.
type Partial struct {
	Harmonic  float64
	Amplitude float64
}

// Sine is a pure tone.
var Sine = []Partial{{Harmonic: 1, Amplitude: 1}}

// Buzzy adds odd harmonics for a rougher tone.
var Buzzy = []Partial{{Harmonic: 1, Amplitude: 1}, {Harmonic: 3, Amplitude: 0.3}, {Harmonic: 5, Amplitude: 0.15}}

// Note is one tone that glides from From to To semitones above the root.
type Note struct {
	From, To float64
	Onset    time.Duration
	Length   time.Duration
	Decay    time.Duration
	Timbre   []Partial
}

// Amplitude converts decibels relative to full scale to a linear amplitude.
func Amplitude(db float64) float64 { return math.Pow(10, db/decibelsPerDecade) }

// Hz returns the frequency of a MIDI note number, which may be fractional.
func Hz(note float64) float64 { return concertAHz * math.Exp2((note-concertANote)/semitonesPerOctave) }

// MIDINote returns the MIDI note number of a frequency.
func MIDINote(hz float64) float64 { return concertANote + semitonesPerOctave*math.Log2(hz/concertAHz) }

// Render mixes a score on the root MIDI note into one buffer normalized to peak at peakDB.
func Render(score []Note, root int, peakDB float64) []float32 {
	buf := make([]float64, end(score))
	for _, n := range score {
		addNote(buf[frames(n.Onset):], n, float64(root))
	}
	return normalize(buf, Amplitude(peakDB))
}

func frames(d time.Duration) int { return int(d.Seconds() * SampleRate) }

func end(score []Note) int {
	last := 0
	for _, n := range score {
		last = max(last, frames(n.Onset)+frames(n.Length))
	}
	return last
}

func addNote(dst []float64, n Note, root float64) {
	count := frames(n.Length)
	phase := 0.0
	for i := range count {
		progress := float64(i) / float64(count)
		phase += 2 * math.Pi * Hz(root+n.From+(n.To-n.From)*progress) / SampleRate
		dst[i] += envelope(i, count, n.Decay) * timbre(phase, n.Timbre)
	}
}

func envelope(i, count int, decay time.Duration) float64 {
	rise := min(1, float64(i)/float64(frames(attack)))
	fall := min(1, float64(count-1-i)/float64(frames(release)))
	return rise * fall * math.Exp(-float64(i)/SampleRate/decay.Seconds())
}

func timbre(phase float64, partials []Partial) float64 {
	sum := 0.0
	for _, p := range partials {
		sum += p.Amplitude * math.Sin(p.Harmonic*phase)
	}
	return sum
}

func normalize(buf []float64, peak float64) []float32 {
	loudest := 0.0
	for _, x := range buf {
		loudest = max(loudest, math.Abs(x))
	}
	out := make([]float32, len(buf))
	for i, x := range buf {
		out[i] = float32(x * peak / loudest)
	}
	return out
}
