// Package playback streams float32 PCM to PipeWire through pw-cat.
package playback

import (
	"encoding/binary"
	"errors"
	"math"
	"os"
	"os/exec"
	"strconv"
	"time"

	"golang.org/x/sys/unix"

	"meknik/synth"
)

// PipeBytes bounds the pipe to pw-cat so that queued audio stays short.
const PipeBytes = 4096

// StallTimeout is how long a write may wait on a player that stopped reading.
const StallTimeout = time.Second

var pwCatArgs = []string{
	"--playback", "--raw",
	"--format", "f32",
	"--rate", strconv.Itoa(synth.SampleRate),
	"--channels", "1",
	"--properties", "application.name=meknik",
	"-",
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

// Command returns the pw-cat invocation that plays raw mono float32 PCM from stdin.
func Command() *exec.Cmd { return exec.Command("pw-cat", pwCatArgs...) }

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

// Close ends the stream and waits for pw-cat to drain and exit.
func (s *Stream) Close() error { return errors.Join(s.pipe.Close(), s.cmd.Wait()) }
