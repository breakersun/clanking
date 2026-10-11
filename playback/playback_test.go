package playback_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/chrchr/clanking/pan"
	"github.com/chrchr/clanking/playback"
)

func TestEncodeWritesLittleEndianFloat32(t *testing.T) {
	want := []byte{0x00, 0x00, 0x80, 0x3f}
	if got := playback.Encode(nil, []float32{1}); !bytes.Equal(want, got) {
		t.Errorf("Encode(1) = %x, want %x", got, want)
	}
}

func TestEncodeAppendsToDestination(t *testing.T) {
	if got := len(playback.Encode(make([]byte, 2), []float32{0, 0})); got != 10 {
		t.Errorf("len = %d, want 10", got)
	}
}

func TestWriteFailsWhenPlayerStopsReading(t *testing.T) {
	cmd := exec.Command("sleep", "10")
	stream, err := playback.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	if _, err := stream.Write(make([]byte, 4*playback.PipeBytes)); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Errorf("Write error = %v, want %v", err, os.ErrDeadlineExceeded)
	}
}

func TestCommandPlaysStereo(t *testing.T) {
	args := playback.Command().Args
	i := slices.Index(args, "--channels")
	if got := args[i+1]; got != strconv.Itoa(pan.Channels) {
		t.Errorf("--channels %s, want %d", got, pan.Channels)
	}
}

func TestCommandDefaultsToPwCat(t *testing.T) {
	t.Setenv("CLANKING_PLAYER", "")
	if got := playback.Command().Path; !strings.HasSuffix(got, "pw-cat") {
		t.Errorf("Path = %q, want pw-cat", got)
	}
}

func TestCommandPacatOverride(t *testing.T) {
	t.Setenv("CLANKING_PLAYER", "pacat")
	cmd := playback.Command()
	if got := cmd.Path; !strings.HasSuffix(got, "pacat") {
		t.Errorf("Path = %q, want pacat", got)
	}
	if !slices.Contains(cmd.Args, "float32le") {
		t.Errorf("Args = %v, want float32le format", cmd.Args)
	}
}

func TestCommandFallsBackToPacat(t *testing.T) {
	t.Setenv("CLANKING_PLAYER", "")
	dir := t.TempDir()
	pacat := filepath.Join(dir, "pacat")
	if err := os.WriteFile(pacat, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	cmd := playback.Command()
	if got := cmd.Path; !strings.HasSuffix(got, "pacat") {
		t.Errorf("Path = %q, want pacat fallback", got)
	}
}

func TestCommandNoPlayerKeepsPwCatName(t *testing.T) {
	t.Setenv("CLANKING_PLAYER", "")
	t.Setenv("PATH", t.TempDir())
	if got := playback.Command().Path; !strings.HasSuffix(got, "pw-cat") {
		t.Errorf("Path = %q, want pw-cat", got)
	}
}
