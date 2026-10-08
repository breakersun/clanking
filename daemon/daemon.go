// Package daemon runs the single-instance audio loop that plays cues.
package daemon

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"meknik/control"
	"meknik/grid"
	"meknik/mixer"
	"meknik/pan"
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

// Play schedules queued sounds on the grid and mixes them into out block by block, applying queued settings, until idle passes with no new sound, or forever with NeverIdle.
func Play(m *mixer.Mixer, s *grid.Scheduler, sounds <-chan mixer.Sound, settings <-chan control.Settings, out io.Writer, idle time.Duration, now func() time.Time, started func(mixer.Sound)) error {
	block := make([]float32, pan.Channels*BlockFrames)
	var pcm []byte
	last := now()
	for idle == NeverIdle || now().Sub(last) < idle {
		last = drain(m, s, sounds, settings, last, now)
		for _, st := range s.Due(BlockFrames) {
			m.AddAt(st.Sound, st.Offset)
			started(st.Sound)
		}
		m.Fill(block)
		pcm = playback.Encode(pcm[:0], block)
		if _, err := out.Write(pcm); err != nil {
			return err
		}
	}
	return nil
}

func drain(m *mixer.Mixer, s *grid.Scheduler, sounds <-chan mixer.Sound, settings <-chan control.Settings, last time.Time, now func() time.Time) time.Time {
	for {
		select {
		case snd := <-sounds:
			if !s.Offer(snd) {
				slog.Warn("grid queue full, sound dropped", "cue", snd.Cue)
			}
			last = now()
		case st := <-settings:
			m.SetGain(st.Gain())
			s.SetTempo(st.TempoBPM)
		default:
			return last
		}
	}
}
