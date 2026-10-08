// Package mixer sums active cue voices into output blocks under a soft limiter.
package mixer

import (
	"math"
	"slices"

	"meknik/pan"
	"meknik/synth"
)

// MaxVoices caps how many sounds play at once.
const MaxVoices = 8

// Sound is a rendered cue queued for playback at a stereo pan.
type Sound struct {
	Samples []float32
	Cue     synth.Cue
	Pan     float64
}

func (s Sound) tick() bool { return s.Cue == synth.Tick }

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

// New returns a mixer that applies gain after a tanh limiter held below ceiling, so gain shifts every level.
func New(gain, ceiling float64) *Mixer { return &Mixer{gain: gain, ceiling: ceiling} }

// Add starts a sound now.
func (m *Mixer) Add(s Sound) { m.AddAt(s, 0) }

// AddAt starts a sound offset frames into the next block; when the pool is full a tick is dropped and a cue replaces the oldest tick.
func (m *Mixer) AddAt(s Sound, offset int) {
	if len(m.voices) < MaxVoices {
		m.voices = append(m.voices, &voice{Sound: s, pos: -offset})
		return
	}
	if s.tick() {
		return
	}
	if i := slices.IndexFunc(m.voices, func(v *voice) bool { return v.tick() }); i >= 0 {
		m.voices = append(slices.Delete(m.voices, i, i+1), &voice{Sound: s, pos: -offset})
	}
}

// Fill renders the next block of interleaved stereo frames and retires finished voices.
func (m *Mixer) Fill(block []float32) {
	clear(block)
	frames := len(block) / pan.Channels
	for _, v := range m.voices {
		left, right := pan.Gains(v.Pan)
		for i := range frames {
			if j := v.pos + i; j >= 0 && j < len(v.Samples) {
				block[pan.Channels*i] += float32(left) * v.Samples[j]
				block[pan.Channels*i+1] += float32(right) * v.Samples[j]
			}
		}
		v.pos += frames
	}
	m.voices = slices.DeleteFunc(m.voices, func(v *voice) bool { return v.pos >= len(v.Samples) })
	for i, x := range block {
		block[i] = float32(m.gain * m.ceiling * math.Tanh(float64(x)/m.ceiling))
	}
}

// SetGain changes the gain applied after the limiter.
func (m *Mixer) SetGain(gain float64) { m.gain = gain }

// Voices reports how many sounds are still playing.
func (m *Mixer) Voices() int { return len(m.voices) }
