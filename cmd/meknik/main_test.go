package main

import (
	"errors"
	"os"
	"testing"
	"time"

	"meknik/daemon"
	"meknik/internal/approx"
	"meknik/mixer"
	"meknik/synth"
)

const (
	tolerance = 1e-9
	stallWait = time.Second
)

func TestParseGainDefaultsToUnity(t *testing.T) {
	if got, _ := parseGain(""); got != 1 {
		t.Errorf("parseGain(\"\") = %v, want 1", got)
	}
}

func TestParseGainConvertsDecibels(t *testing.T) {
	if got, _ := parseGain("-6"); !approx.Equal(synth.Amplitude(-6), got, tolerance) {
		t.Errorf("parseGain(-6) = %v, want %v", got, synth.Amplitude(-6))
	}
}

func TestParseGainRejectsGarbage(t *testing.T) {
	if _, err := parseGain("loud"); err == nil {
		t.Error("parseGain(loud) error = nil, want an error")
	}
}

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
		enqueue(make(chan mixer.Sound), newRNG())(synth.Done)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(stallWait):
		t.Error("enqueue blocked on a full queue")
	}
}

func TestDaemonCommandSendsStderrToLog(t *testing.T) {
	logFile := os.Stdout
	if got := daemonCommand("/bin/meknik", logFile).Stderr; got != logFile {
		t.Errorf("Stderr = %v, want %v", got, logFile)
	}
}

func TestStatePathLivesUnderStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/state")
	if got, _ := statePath(logName); got != "/state/meknik/meknik.log" {
		t.Errorf("statePath = %q, want /state/meknik/meknik.log", got)
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

func TestRunPathLivesInSharedRunDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/state")
	if got, _ := runPath(socketName); got != "/state/meknik/run/meknik.sock" {
		t.Errorf("runPath = %q, want /state/meknik/run/meknik.sock", got)
	}
}
