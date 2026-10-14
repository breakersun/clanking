// Package playback streams float32 PCM to PipeWire through pw-cat.
package playback

import (
	"encoding/binary"
	"errors"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"strconv"
	"time"

	"golang.org/x/sys/unix"

	"github.com/chrchr/clanking/pan"
	"github.com/chrchr/clanking/synth"
)

// PipeBytes bounds the pipe to pw-cat so that queued audio stays short.
const PipeBytes = 4096

// StallTimeout is how long a write may wait on a player that stopped reading.
const StallTimeout = time.Second

// PlayerEnv names the environment variable that overrides the audio player binary.
const PlayerEnv = "CLANKING_PLAYER"

var pwCatArgs = []string{
	"--playback", "--raw",
	"--format", "f32",
	"--rate", strconv.Itoa(synth.SampleRate),
	"--channels", strconv.Itoa(pan.Channels),
	"--properties", "application.name=clanking",
	"-",
}

// pacat reads raw PCM from stdin by default and does not accept "-" for it.
// A short --latency-msec keeps cues in sync on buffered sinks such as WSLg's
// RDP sound device.
var paCatArgs = []string{
	"--playback", "--raw",
	"--format", "float32le",
	"--rate", strconv.Itoa(synth.SampleRate),
	"--channels", strconv.Itoa(pan.Channels),
	"--latency-msec=60",
	"--property=application.name=clanking",
}

// player returns the audio player binary and its raw-PCM arguments. It defaults
// to pw-cat (PipeWire) and supports pacat (PulseAudio), as on WSLg, selected
// through PlayerEnv. When pw-cat is not installed but pacat is, as on WSLg,
// it falls back to pacat.
func player() (string, []string) {
	if name := os.Getenv(PlayerEnv); name == "pacat" {
		return "pacat", paCatArgs
	}
	if _, err := exec.LookPath("pw-cat"); err != nil {
		if _, err := exec.LookPath("pacat"); err == nil {
			return "pacat", paCatArgs
		}
	}
	return "pw-cat", pwCatArgs
}

// Encode appends samples to dst as little-endian float32 bytes.
func Encode(dst []byte, samples []float32) []byte {
	for _, x := range samples {
		dst = binary.LittleEndian.AppendUint32(dst, math.Float32bits(x))
	}
	return dst
}

// Stream is a running pw-cat fed through a short pipe.
type Stream struct {
	cmd  *exec.Cmd
	pipe *os.File
}

// Command returns the invocation that plays raw interleaved stereo float32 PCM
// from stdin: pw-cat by default, or the player named by PlayerEnv.
func Command() *exec.Cmd {
	name, args := player()
	return exec.Command(name, args...)
}

// Start launches a player reading raw PCM from a short pipe.
func Start(cmd *exec.Cmd) (*Stream, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	if err := shrink(w); err != nil {
		return nil, err
	}
	cmd.Stdin = r
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Stream{cmd: cmd, pipe: w}, r.Close()
}

// Write sends encoded PCM to the player, failing if it stalls past StallTimeout.
func (s *Stream) Write(p []byte) (int, error) {
	if err := s.pipe.SetWriteDeadline(time.Now().Add(StallTimeout)); err != nil {
		return 0, err
	}
	return s.pipe.Write(p)
}

func shrink(pipe *os.File) error {
	raw, err := pipe.SyscallConn()
	if err != nil {
		return err
	}
	var fcntlErr error
	if err := raw.Control(func(fd uintptr) { _, fcntlErr = unix.FcntlInt(fd, unix.F_SETPIPE_SZ, PipeBytes) }); err != nil {
		return err
	}
	return fcntlErr
}

// Kill terminates a player that stopped reading, without waiting for a drain.
func (s *Stream) Kill() error {
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	return errors.Join(s.pipe.Close(), s.cmd.Wait())
}

// Close ends the stream and waits for pw-cat to drain and exit.
func (s *Stream) Close() error { return errors.Join(s.pipe.Close(), s.cmd.Wait()) }

// Player streams PCM to the audio player, replacing the player when its stream
// goes stale, as on WSLg after sleep and resume leaves the old stream wedged.
type Player struct {
	stream *Stream
}

// NewPlayer returns a Player that launches the audio player on first write.
func NewPlayer() *Player { return new(Player) }

// Write sends encoded PCM to the player. A stalled player is killed and
// replaced once per write, so one stale stream cannot stop the daemon; a
// second consecutive stall fails the write.
func (p *Player) Write(b []byte) (int, error) {
	if p.stream == nil {
		if err := p.start(); err != nil {
			return 0, err
		}
	}
	n, err := p.stream.Write(b)
	if err == nil {
		return n, nil
	}
	p.recycle()
	if err := p.start(); err != nil {
		return n, err
	}
	m, err := p.stream.Write(b[n:])
	return n + m, err
}

// Close ends the current player and waits for it to drain and exit, killing it
// when it does not, as a player wedged on a stale stream never sees end of input.
func (p *Player) Close() error {
	if p.stream == nil {
		return nil
	}
	s := p.stream
	p.stream = nil
	done := make(chan error, 1)
	go func() { done <- s.Close() }()
	select {
	case err := <-done:
		return err
	case <-time.After(StallTimeout):
		return errors.Join(s.Kill(), <-done)
	}
}

func (p *Player) start() error {
	s, err := Start(Command())
	if err != nil {
		return err
	}
	p.stream = s
	return nil
}

func (p *Player) recycle() {
	s := p.stream
	p.stream = nil
	slog.Warn("stalled audio player replaced")
	_ = s.Kill()
}
