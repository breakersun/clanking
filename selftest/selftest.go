// Package selftest plays every cue once through the daemon, as a check that sound works.
package selftest

import (
	"errors"
	"math/rand/v2"
	"time"

	"github.com/chrchr/clanking/control"
	"github.com/chrchr/clanking/statedir"
	"github.com/chrchr/clanking/synth"
	"github.com/chrchr/clanking/transport"
)

const cuePause = 400 * time.Millisecond

// ErrMuted reports a self-test refused because clanking is muted.
var ErrMuted = errors.New("muted; unmute to run the self-test")

// Run plays every cue once through send, waiting for each to finish, and refuses while muted.
func Run(status func() (control.Settings, error), send func(synth.Cue) error, sleep func(time.Duration)) error {
	settings, err := status()
	if err != nil {
		return err
	}
	if settings.Muted {
		return ErrMuted
	}
	rng := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	for _, c := range synth.Cues {
		if err := send(c); err != nil {
			return err
		}
		samples, _ := synth.Sound(c, control.DefaultRootNote, rng)
		sleep(time.Duration(len(samples))*time.Second/synth.SampleRate + cuePause)
	}
	return nil
}

// RunDaemon runs the self-test against the daemon, starting one with spawn when none answers.
func RunDaemon(spawn func() error) error {
	controlPath, err := statedir.RunPath(statedir.ControlSocket)
	if err != nil {
		return err
	}
	cuePath, err := statedir.RunPath(statedir.CueSocket)
	if err != nil {
		return err
	}
	status := func() (control.Settings, error) {
		s, _, err := transport.Control(controlPath, control.Request{Command: control.Status}, spawn)
		return s, err
	}
	return Run(status, func(c synth.Cue) error { return transport.Send(cuePath, c, "") }, time.Sleep)
}
