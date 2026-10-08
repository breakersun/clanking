package control_test

import (
	"errors"
	"path/filepath"
	"testing"

	"meknik/control"
	"meknik/internal/approx"
)

const tolerance = 1e-9

var errDisk = errors.New("disk full")

func TestParseAcceptsRequests(t *testing.T) {
	cases := map[string]control.Request{
		"mute":        {Command: control.Mute},
		"unmute\n":    {Command: control.Unmute},
		"status":      {Command: control.Status},
		"volume -6":   {Command: control.Volume, VolumeDB: -6},
		"volume 24\n": {Command: control.Volume, VolumeDB: 24},
	}
	for line, want := range cases {
		t.Run(line, func(t *testing.T) {
			if got, _ := control.Parse(line); got != want {
				t.Errorf("Parse(%q) = %+v, want %+v", line, got, want)
			}
		})
	}
}

func TestParseRejectsInvalidRequests(t *testing.T) {
	for _, line := range []string{"", "louder", "volume", "volume loud", "volume NaN", "volume -Inf", "volume 24.5", "mute now"} {
		t.Run(line, func(t *testing.T) {
			if _, err := control.Parse(line); !errors.Is(err, control.ErrInvalid) {
				t.Errorf("Parse(%q) error = %v, want %v", line, err, control.ErrInvalid)
			}
		})
	}
}

func TestRequestStringParsesBack(t *testing.T) {
	want := control.Request{Command: control.Volume, VolumeDB: -7.5}
	if got, _ := control.Parse(want.String()); got != want {
		t.Errorf("Parse(String()) = %+v, want %+v", got, want)
	}
}

func TestSettingsStringReportsStatus(t *testing.T) {
	if got := (control.Settings{VolumeDB: -6, Muted: true}).String(); got != "volume -6 dB, muted" {
		t.Errorf("String = %q, want %q", got, "volume -6 dB, muted")
	}
}

func TestGainIsSilentWhenMuted(t *testing.T) {
	if got := (control.Settings{Muted: true}).Gain(); got != 0 {
		t.Errorf("Gain = %v, want 0", got)
	}
}

func TestGainFollowsVolume(t *testing.T) {
	if got := (control.Settings{VolumeDB: -20}).Gain(); !approx.Equal(0.1, got, tolerance) {
		t.Errorf("Gain = %v, want 0.1", got)
	}
}

func TestApplyVolumeKeepsMute(t *testing.T) {
	got := control.Settings{Muted: true}.Apply(control.Request{Command: control.Volume, VolumeDB: -3})
	if want := (control.Settings{VolumeDB: -3, Muted: true}); got != want {
		t.Errorf("Apply = %+v, want %+v", got, want)
	}
}

func TestApplyUnmute(t *testing.T) {
	got := control.Settings{VolumeDB: -3, Muted: true}.Apply(control.Request{Command: control.Unmute})
	if want := (control.Settings{VolumeDB: -3}); got != want {
		t.Errorf("Apply = %+v, want %+v", got, want)
	}
}

func TestHandleSavesChangedSettings(t *testing.T) {
	var saved control.Settings
	c := control.NewController(control.Settings{}, func(s control.Settings) error { saved = s; return nil }, func(control.Settings) {})
	_, _ = c.Handle(control.Request{Command: control.Mute})
	if want := (control.Settings{Muted: true}); saved != want {
		t.Errorf("saved = %+v, want %+v", saved, want)
	}
}

func TestHandleAppliesChangedSettings(t *testing.T) {
	var applied control.Settings
	c := control.NewController(control.Settings{}, func(control.Settings) error { return nil }, func(s control.Settings) { applied = s })
	_, _ = c.Handle(control.Request{Command: control.Volume, VolumeDB: -9})
	if want := (control.Settings{VolumeDB: -9}); applied != want {
		t.Errorf("applied = %+v, want %+v", applied, want)
	}
}

func TestHandleStatusSavesNothing(t *testing.T) {
	saves := 0
	c := control.NewController(control.Settings{}, func(control.Settings) error { saves++; return nil }, func(control.Settings) {})
	_, _ = c.Handle(control.Request{Command: control.Status})
	if saves != 0 {
		t.Errorf("saves = %d, want 0", saves)
	}
}

func TestHandleKeepsSettingsWhenSaveFails(t *testing.T) {
	c := control.NewController(control.Settings{}, func(control.Settings) error { return errDisk }, func(control.Settings) {})
	_, _ = c.Handle(control.Request{Command: control.Mute})
	if got, _ := c.Handle(control.Request{Command: control.Status}); got != (control.Settings{}) {
		t.Errorf("settings after failed save = %+v, want unchanged", got)
	}
}

func TestHandleReportsSaveFailure(t *testing.T) {
	c := control.NewController(control.Settings{}, func(control.Settings) error { return errDisk }, func(control.Settings) {})
	if _, err := c.Handle(control.Request{Command: control.Mute}); !errors.Is(err, errDisk) {
		t.Errorf("Handle error = %v, want %v", err, errDisk)
	}
}

func TestLoadWithoutFileGivesDefaults(t *testing.T) {
	if got, _ := control.Load(filepath.Join(t.TempDir(), "mixer.json")); got != (control.Settings{}) {
		t.Errorf("Load = %+v, want defaults", got)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixer.json")
	want := control.Settings{VolumeDB: -4.5, Muted: true}
	_ = control.Save(path, want)
	if got, _ := control.Load(path); got != want {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}
