package selftest_test

import (
	"errors"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/chrchr/clanking/control"
	"github.com/chrchr/clanking/selftest"
	"github.com/chrchr/clanking/synth"
)

const cuePause = 400 * time.Millisecond

var (
	errStatus = errors.New("status failed")
	errSend   = errors.New("send failed")
)

func unmuted() (control.Settings, error) { return control.Settings{}, nil }
func muted() (control.Settings, error)   { return control.Settings{Muted: true}, nil }
func noSleep(time.Duration)              {}

func record(sent *[]synth.Cue) func(synth.Cue) error {
	return func(c synth.Cue) error { *sent = append(*sent, c); return nil }
}

func TestRunSendsCuesInOrder(t *testing.T) {
	var sent []synth.Cue
	_ = selftest.Run(unmuted, record(&sent), noSleep)
	if !slices.Equal(sent, synth.Cues) {
		t.Errorf("sent = %v, want %v", sent, synth.Cues)
	}
}

func TestRunWaitsForEachCueAndAPause(t *testing.T) {
	var waits []time.Duration
	_ = selftest.Run(unmuted, record(new([]synth.Cue)), func(d time.Duration) { waits = append(waits, d) })
	samples, _ := synth.Sound(synth.Cues[len(synth.Cues)-1], rand.New(rand.NewPCG(1, 2)))
	if want := time.Duration(len(samples))*time.Second/synth.SampleRate + cuePause; waits[len(waits)-1] != want {
		t.Errorf("last wait = %v, want %v", waits[len(waits)-1], want)
	}
}

func TestRunRefusesWhileMuted(t *testing.T) {
	if err := selftest.Run(muted, record(new([]synth.Cue)), noSleep); !errors.Is(err, selftest.ErrMuted) {
		t.Errorf("Run error = %v, want %v", err, selftest.ErrMuted)
	}
}

func TestRunSendsNothingWhileMuted(t *testing.T) {
	var sent []synth.Cue
	_ = selftest.Run(muted, record(&sent), noSleep)
	if len(sent) != 0 {
		t.Errorf("sent = %v, want none", sent)
	}
}

func TestRunReturnsStatusError(t *testing.T) {
	status := func() (control.Settings, error) { return control.Settings{}, errStatus }
	if err := selftest.Run(status, record(new([]synth.Cue)), noSleep); !errors.Is(err, errStatus) {
		t.Errorf("Run error = %v, want %v", err, errStatus)
	}
}

func TestRunStopsOnSendError(t *testing.T) {
	sends := 0
	_ = selftest.Run(unmuted, func(synth.Cue) error { sends++; return errSend }, noSleep)
	if sends != 1 {
		t.Errorf("sends = %d, want 1", sends)
	}
}
