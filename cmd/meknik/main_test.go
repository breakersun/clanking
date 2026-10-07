package main

import (
	"errors"
	"testing"

	"meknik/internal/approx"
	"meknik/synth"
)

const tolerance = 1e-9

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
