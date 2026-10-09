package synth

import "fmt"

const (
	midiNotes        = 128
	midiOctaveOffset = 1
)

var pitchClassNames = [semitonesPerOctave]string{"C", "C♯", "D", "D♯", "E", "F", "F♯", "G", "G♯", "A", "A♯", "B"}

// NoteName names a MIDI note in scientific pitch notation with sharps, such as D♯5.
func NoteName(note int) string {
	return fmt.Sprintf("%s%d", pitchClassNames[note%semitonesPerOctave], note/semitonesPerOctave-midiOctaveOffset)
}

// ParseNoteName returns the MIDI note that NoteName names name, reporting false for an unknown name.
func ParseNoteName(name string) (int, bool) {
	for note := range midiNotes {
		if NoteName(note) == name {
			return note, true
		}
	}
	return 0, false
}
