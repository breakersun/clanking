package main

import (
	"slices"
	"testing"
	"time"

	"github.com/chrchr/clanking/control"
)

const slowAsk = 50 * time.Millisecond

func TestForwardRepliesInRequestOrder(t *testing.T) {
	requests := make(chan control.Request, 2)
	requests <- control.Request{Command: control.Mute}
	requests <- control.Request{Command: control.Unmute}
	close(requests)
	ask := func(r control.Request) (control.Settings, error) {
		if r.Command == control.Mute {
			time.Sleep(slowAsk)
		}
		return control.Settings{}.Apply(r), nil
	}
	var replies []bool
	forward(requests, ask, func(s control.Settings, _ error) { replies = append(replies, s.Muted) })
	if want := []bool{true, false}; !slices.Equal(replies, want) {
		t.Errorf("replies = %v, want %v", replies, want)
	}
}

func TestDetentSnapsNearZeroToZero(t *testing.T) {
	if got := detent(-0.8); got != 0 {
		t.Errorf("detent(-0.8) = %v, want 0", got)
	}
}

func TestDetentKeepsValuesOutsideIt(t *testing.T) {
	if got := detent(1.5); got != 1.5 {
		t.Errorf("detent(1.5) = %v, want 1.5", got)
	}
}

func TestTempoRequestCarriesBPM(t *testing.T) {
	if got, want := tempoRequest(85), (control.Request{Command: control.Tempo, TempoBPM: 85}); got != want {
		t.Errorf("tempoRequest(85) = %+v, want %+v", got, want)
	}
}

func TestMuteIconWhenMuted(t *testing.T) {
	if got := muteIcon(true); got != "audio-volume-muted-symbolic" {
		t.Errorf("muteIcon(true) = %q, want audio-volume-muted-symbolic", got)
	}
}

func TestMuteIconWhenUnmuted(t *testing.T) {
	if got := muteIcon(false); got != "audio-volume-high-symbolic" {
		t.Errorf("muteIcon(false) = %q, want audio-volume-high-symbolic", got)
	}
}
