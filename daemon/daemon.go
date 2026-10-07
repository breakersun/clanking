// Package daemon runs the single-instance audio loop that plays cues.
package daemon

import (
	"errors"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"meknik/mixer"
	"meknik/playback"
)

// BlockFrames is the number of frames rendered per write.
const BlockFrames = 256

// NeverIdle disables the idle exit.
const NeverIdle time.Duration = 0

const lockMode = 0o600

// Lock takes the single-instance lock, reporting false when another daemon holds it.
func Lock(path string) (*os.File, bool, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, lockMode)
	if err != nil {
		return nil, false, err
	}
	err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return nil, false, f.Close()
	}
	if err != nil {
		return nil, false, errors.Join(err, f.Close())
	}
	return f, true, nil
}

// Play mixes queued sounds into out block by block until idle passes with no new sound, or forever with NeverIdle.
func Play(m *mixer.Mixer, sounds <-chan mixer.Sound, out io.Writer, idle time.Duration, now func() time.Time) error {
	block := make([]float32, BlockFrames)
	var pcm []byte
	last := now()
	for idle == NeverIdle || now().Sub(last) < idle {
		last = drain(m, sounds, last, now)
		m.Fill(block)
		pcm = playback.Encode(pcm[:0], block)
		if _, err := out.Write(pcm); err != nil {
			return err
		}
	}
	return nil
}

func drain(m *mixer.Mixer, sounds <-chan mixer.Sound, last time.Time, now func() time.Time) time.Time {
	for {
		select {
		case s := <-sounds:
			m.Add(s)
			last = now()
		default:
			return last
		}
	}
}
