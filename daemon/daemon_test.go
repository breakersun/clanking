package daemon_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"meknik/daemon"
	"meknik/internal/approx"
	"meknik/mixer"
)

const (
	lockName       = "meknik.lock"
	idle           = 10 * time.Millisecond
	clockStep      = time.Millisecond
	unity          = 1.0
	wideCeiling    = 100.0
	sample         = 0.1
	tolerance      = 1e-4
	bytesPerSample = 4
)

var errBroken = errors.New("pipe broken")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errBroken }

func steppingClock() func() time.Time {
	t := time.Unix(0, 0)
	return func() time.Time {
		t = t.Add(clockStep)
		return t
	}
}

func play(sounds <-chan mixer.Sound) *bytes.Buffer {
	var out bytes.Buffer
	_ = daemon.Play(mixer.New(unity, wideCeiling), sounds, make(chan float64), &out, idle, steppingClock())
	return &out
}

func TestLockExcludesSecondHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), lockName)
	first, _, _ := daemon.Lock(path)
	t.Cleanup(func() { _ = first.Close() })
	if _, held, _ := daemon.Lock(path); held {
		t.Error("second Lock held = true, want false")
	}
}

func TestLockIsFreeAfterRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), lockName)
	first, _, _ := daemon.Lock(path)
	_ = first.Close()
	second, held, _ := daemon.Lock(path)
	t.Cleanup(func() { _ = second.Close() })
	if !held {
		t.Error("Lock after release held = false, want true")
	}
}

func TestPlayStopsWhenIdle(t *testing.T) {
	err := daemon.Play(mixer.New(unity, wideCeiling), make(chan mixer.Sound), make(chan float64), &bytes.Buffer{}, idle, steppingClock())
	if err != nil {
		t.Errorf("Play error = %v, want nil", err)
	}
}

func TestPlayWritesWholeBlocks(t *testing.T) {
	if got := play(make(chan mixer.Sound)).Len() % (daemon.BlockFrames * bytesPerSample); got != 0 {
		t.Errorf("output length remainder = %d, want 0", got)
	}
}

func TestPlayWritesQueuedSound(t *testing.T) {
	sounds := make(chan mixer.Sound, 1)
	sounds <- mixer.Sound{Samples: []float32{sample}}
	got := float64(math.Float32frombits(binary.LittleEndian.Uint32(play(sounds).Bytes())))
	if !approx.Equal(sample, got, tolerance) {
		t.Errorf("first sample = %v, want %v", got, sample)
	}
}

func TestPlayReportsWriteFailure(t *testing.T) {
	err := daemon.Play(mixer.New(unity, wideCeiling), make(chan mixer.Sound), make(chan float64), failingWriter{}, idle, steppingClock())
	if !errors.Is(err, errBroken) {
		t.Errorf("Play error = %v, want %v", err, errBroken)
	}
}

type brokenAfter struct{ writes int }

func (w *brokenAfter) Write(p []byte) (int, error) {
	if w.writes == 0 {
		return 0, errBroken
	}
	w.writes--
	return len(p), nil
}

func TestPlayWithNeverIdleKeepsPlaying(t *testing.T) {
	out := &brokenAfter{writes: 100}
	err := daemon.Play(mixer.New(unity, wideCeiling), make(chan mixer.Sound), make(chan float64), out, daemon.NeverIdle, steppingClock())
	if !errors.Is(err, errBroken) {
		t.Errorf("Play error = %v, want %v", err, errBroken)
	}
}

func TestLockRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, lockName)
	_ = os.Symlink(filepath.Join(dir, "target"), path)
	if _, _, err := daemon.Lock(path); err == nil {
		t.Error("Lock error = nil, want an error for a symlinked lock")
	}
}

func TestPlayAppliesQueuedGain(t *testing.T) {
	sounds := make(chan mixer.Sound, 1)
	sounds <- mixer.Sound{Samples: []float32{sample}}
	gains := make(chan float64, 1)
	gains <- 0
	var out bytes.Buffer
	_ = daemon.Play(mixer.New(unity, wideCeiling), sounds, gains, &out, idle, steppingClock())
	if got := math.Float32frombits(binary.LittleEndian.Uint32(out.Bytes())); got != 0 {
		t.Errorf("first sample = %v, want 0", got)
	}
}
