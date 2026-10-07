// Package mixer sums active cue voices into output blocks under a soft limiter.
package mixer

import (
	"math"
	"slices"
)

// MaxVoices caps how many sounds play at once.
const MaxVoices = 8

// Sound is a rendered cue queued for playback.
type Sound struct {
	Samples []float32
	Tick    bool
}

type voice struct {
	Sound
	pos int
}

// Mixer holds the active voices and the output gain staging.
type Mixer struct {
	voices  []*voice
	gain    float64
	ceiling float64
}

// New returns a mixer that applies gain before a tanh limiter held below ceiling.
func New(gain, ceiling float64) *Mixer { return &Mixer{gain: gain, ceiling: ceiling} }

// Add starts a sound; when the pool is full a tick is dropped and a cue replaces the oldest tick.
func (m *Mixer) Add(s Sound) {
	if len(m.voices) < MaxVoices {
		m.voices = append(m.voices, &voice{Sound: s})
		return
	}
	if s.Tick {
		return
	}
	if i := slices.IndexFunc(m.voices, func(v *voice) bool { return v.Tick }); i >= 0 {
		m.voices = append(slices.Delete(m.voices, i, i+1), &voice{Sound: s})
	}
}

// Fill renders the next block and retires finished voices.
func (m *Mixer) Fill(block []float32) {
	clear(block)
	for _, v := range m.voices {
		n := min(len(block), len(v.Samples)-v.pos)
		for i := range n {
			block[i] += v.Samples[v.pos+i]
		}
		v.pos += n
	}
	m.voices = slices.DeleteFunc(m.voices, func(v *voice) bool { return v.pos == len(v.Samples) })
	for i, x := range block {
		block[i] = float32(m.ceiling * math.Tanh(m.gain*float64(x)/m.ceiling))
	}
}

// Voices reports how many sounds are still playing.
func (m *Mixer) Voices() int { return len(m.voices) }
