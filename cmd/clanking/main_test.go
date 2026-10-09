package main

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chrchr/clanking/control"
	"github.com/chrchr/clanking/daemon"
	"github.com/chrchr/clanking/mixer"
	"github.com/chrchr/clanking/pan"
	"github.com/chrchr/clanking/synth"
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
		enqueue(make(chan mixer.Sound), newRNG(), defaultRoot)(synth.Done, "")
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
	want := "usage: clanking play <cue>\ncues: tick spawn return failed permission ask reply milestone noted done"
	if got := playUsage(); got != want {
		t.Errorf("playUsage = %q, want %q", got, want)
	}
}

func TestAnnouncePublishesStartedCue(t *testing.T) {
	var published []string
	announce(func(line string) { published = append(published, line) })(mixer.Sound{Cue: synth.Done})
	if want := []string{"cue done"}; !slices.Equal(published, want) {
		t.Errorf("published = %q, want %q", published, want)
	}
}

func TestStatusLineJoinsSettingsAndGrid(t *testing.T) {
	got := statusLine(control.Settings{VolumeDB: -6, Muted: true, TempoBPM: 120, RootNote: control.DefaultRootNote}, "in 32nds, 3 queued, 41 dropped")
	if want := "volume -6 dB, muted, root D5, 120 bpm in 32nds, 3 queued, 41 dropped"; got != want {
		t.Errorf("statusLine = %q, want %q", got, want)
	}
}

func TestRunRejectsOutOfRangeTempoBeforeContactingDaemon(t *testing.T) {
	if err := run([]string{"tempo", "300"}); !errors.Is(err, control.ErrInvalid) {
		t.Errorf("run error = %v, want %v", err, control.ErrInvalid)
	}
}

func TestUsageListsTempo(t *testing.T) {
	if !strings.Contains(errUsage.Error(), "| tempo <bpm>") {
		t.Errorf("usage = %q, want it to list tempo", errUsage)
	}
}

func TestRunRejectsOutOfRangeRootBeforeContactingDaemon(t *testing.T) {
	if err := run([]string{"root", "100"}); !errors.Is(err, control.ErrInvalid) {
		t.Errorf("run error = %v, want %v", err, control.ErrInvalid)
	}
}

func TestUsageListsRoot(t *testing.T) {
	if !strings.Contains(errUsage.Error(), "| root <midi>") {
		t.Errorf("usage = %q, want it to list root", errUsage)
	}
}

func defaultRoot() int { return control.DefaultRootNote }

func TestEnqueueRendersAtTheCurrentRoot(t *testing.T) {
	sounds := make(chan mixer.Sound, 1)
	enqueue(sounds, newRNG(), func() int { return control.MinRootNote })(synth.Done, "")
	want, _ := synth.Sound(synth.Done, control.MinRootNote, newRNG())
	if got := (<-sounds).Samples; !slices.Equal(got, want) {
		t.Error("enqueued samples differ from Done rendered at the current root")
	}
}

func TestEnqueuePansByCwd(t *testing.T) {
	sounds := make(chan mixer.Sound, 1)
	enqueue(sounds, newRNG(), defaultRoot)(synth.Done, "/home/rc/clanking")
	if got := (<-sounds).Pan; got != pan.Of("/home/rc/clanking") {
		t.Errorf("Pan = %v, want %v", got, pan.Of("/home/rc/clanking"))
	}
}
