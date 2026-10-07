// Package playback streams float32 PCM to PipeWire through pw-cat.
package playback

import (
	"encoding/binary"
	"errors"
	"math"
	"os"
	"os/exec"
	"strconv"

	"golang.org/x/sys/unix"

	"meknik/synth"
)

// PipeBytes bounds the pipe to pw-cat so that queued audio stays short.
const PipeBytes = 4096

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

// Start launches pw-cat reading raw PCM from a short pipe.
func Start() (*Stream, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	if _, err := unix.FcntlInt(w.Fd(), unix.F_SETPIPE_SZ, PipeBytes); err != nil {
		return nil, err
	}
	cmd := exec.Command("pw-cat", pwCatArgs...)
	cmd.Stdin = r
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Stream{cmd: cmd, pipe: w}, r.Close()
}

// Write sends encoded PCM to pw-cat.
func (s *Stream) Write(p []byte) (int, error) { return s.pipe.Write(p) }

// Close ends the stream and waits for pw-cat to drain and exit.
func (s *Stream) Close() error { return errors.Join(s.pipe.Close(), s.cmd.Wait()) }
