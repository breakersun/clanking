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

// fakePlayer writes a player script whose first instance stalls and later
// instances drain stdin, counting instances in a file next to the script.
func fakePlayer(t *testing.T, firstN int) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"PATH=/usr/bin:/bin\n" +
		"dirname=$(dirname \"$0\")\n" +
		"n=$(cat \"$dirname/count\" 2>/dev/null || echo 0)\n" +
		"n=$((n+1)); echo \"$n\" > \"$dirname/count\"\n" +
		"if [ \"$n\" -le " + strconv.Itoa(firstN) + " ]; then sleep 30; else exec cat >/dev/null; fi\n"
	if err := os.WriteFile(filepath.Join(dir, "pacat"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLANKING_PLAYER", "pacat")
	t.Setenv("PATH", dir)
}

func spawnCount(t *testing.T) int {
	t.Helper()
	dir := os.Getenv("PATH")
	b, err := os.ReadFile(filepath.Join(dir, "count"))
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPlayerWriteReplacesStalledPlayer(t *testing.T) {
	fakePlayer(t, 1)
	p := playback.NewPlayer()
	t.Cleanup(func() { _ = p.Close() })
	if _, err := p.Write(make([]byte, 4*playback.PipeBytes)); err != nil {
		t.Errorf("Write error = %v, want recovery after player replacement", err)
	}
	if n := spawnCount(t); n != 2 {
		t.Errorf("player spawns = %d, want 2", n)
	}
}

func TestPlayerWriteFailsAfterTwoStalls(t *testing.T) {
	fakePlayer(t, 2)
	p := playback.NewPlayer()
	t.Cleanup(func() { _ = p.Close() })
	if _, err := p.Write(make([]byte, 4*playback.PipeBytes)); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Errorf("Write error = %v, want %v", err, os.ErrDeadlineExceeded)
	}
	if n := spawnCount(t); n != 2 {
		t.Errorf("player spawns = %d, want 2", n)
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
	t.Setenv("PATH", t.TempDir())
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
