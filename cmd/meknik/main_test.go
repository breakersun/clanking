package main

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"meknik/control"
	"meknik/daemon"
	"meknik/mixer"
	"meknik/synth"
)

const stallWait = time.Second

func TestRunRejectsUnknownCommand(t *testing.T) {
	if err := run([]string{"bogus"}); !errors.Is(err, errUsage) {
		t.Errorf("run error = %v, want %v", err, errUsage)
	}
}

func TestPlayRejectsUnknownCue(t *testing.T) {
	if err := run([]string{"play", "bogus"}); err == nil {
		t.Error("play bogus error = nil, want an error")
	}
}

func TestLimiterCeilingSitsSixDecibelsAboveCues(t *testing.T) {
	if got := limiterCeilingDB - synth.CuePeakDB; got != 6 {
		t.Errorf("ceiling minus cue = %v dB, want 6", got)
	}
}

func TestEnqueueDropsWhenQueueIsFull(t *testing.T) {
	returned := make(chan struct{})
	go func() {
		enqueue(make(chan mixer.Sound), newRNG(), func(string) {})(synth.Done)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(stallWait):
		t.Error("enqueue blocked on a full queue")
	}
}

func TestParseIdleDefaultsToNever(t *testing.T) {
	if got, _ := parseIdle(""); got != daemon.NeverIdle {
		t.Errorf("parseIdle(\"\") = %v, want %v", got, daemon.NeverIdle)
	}
}

func TestParseIdleReadsDuration(t *testing.T) {
	if got, _ := parseIdle("10m"); got != 10*time.Minute {
		t.Errorf("parseIdle(10m) = %v, want 10m", got)
	}
}

func TestRefuseSpawnReportsDisabled(t *testing.T) {
	if err := refuseSpawn(); !errors.Is(err, errSpawnDisabled) {
		t.Errorf("refuseSpawn = %v, want %v", err, errSpawnDisabled)
	}
}

func TestRunRejectsVolumeWithoutLevel(t *testing.T) {
	if err := run([]string{"volume"}); !errors.Is(err, errUsage) {
		t.Errorf("run error = %v, want %v", err, errUsage)
	}
}

func TestRunRejectsInvalidVolumeBeforeContactingDaemon(t *testing.T) {
	if err := run([]string{"volume", "loud"}); !errors.Is(err, control.ErrInvalid) {
		t.Errorf("run error = %v, want %v", err, control.ErrInvalid)
	}
}

func TestUsageListsSelftest(t *testing.T) {
	if !strings.Contains(errUsage.Error(), "| selftest") {
		t.Errorf("usage = %q, want it to list selftest", errUsage)
	}
}

func TestPlayUsageListsEveryCue(t *testing.T) {
	want := "usage: meknik play <cue>\ncues: tick spawn return failed permission ask reply milestone noted done"
	if got := playUsage(); got != want {
		t.Errorf("playUsage = %q, want %q", got, want)
	}
}

func TestEnqueuePublishesAcceptedCue(t *testing.T) {
	var published []string
	enqueue(make(chan mixer.Sound, 1), newRNG(), func(line string) { published = append(published, line) })(synth.Done)
	if want := []string{"cue done"}; !slices.Equal(published, want) {
		t.Errorf("published = %q, want %q", published, want)
	}
}

func TestEnqueuePublishesNothingForDroppedCue(t *testing.T) {
	var published []string
	enqueue(make(chan mixer.Sound), newRNG(), func(line string) { published = append(published, line) })(synth.Done)
	if len(published) != 0 {
		t.Errorf("published = %q, want none", published)
	}
}
