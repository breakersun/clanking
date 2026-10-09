package main

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/chrchr/clanking/control"
	"github.com/chrchr/clanking/synth"
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
	forward(requests, ask, func(_ control.Request, s control.Settings, _ error) { replies = append(replies, s.Muted) })
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

func TestTempoLabelShowsBPM(t *testing.T) {
	if got := tempoLabel(85); got != "85 bpm" {
		t.Errorf("tempoLabel(85) = %q, want 85 bpm", got)
	}
}

func TestRootRequestCarriesNote(t *testing.T) {
	if got, want := rootRequest(70), (control.Request{Command: control.Root, RootNote: 70}); got != want {
		t.Errorf("rootRequest(70) = %+v, want %+v", got, want)
	}
}

func TestRootChangeAuditionsTheTonic(t *testing.T) {
	if got, want := auditions(rootRequest(70), control.Settings{RootNote: 70}, nil), []synth.Cue{synth.Tonic}; !slices.Equal(got, want) {
		t.Errorf("auditions = %v, want %v", got, want)
	}
}

func TestTempoChangeAuditionsFourTicks(t *testing.T) {
	want := []synth.Cue{synth.Tick, synth.Tick, synth.Tick, synth.Tick}
	if got := auditions(tempoRequest(85), control.Settings{TempoBPM: 85}, nil); !slices.Equal(got, want) {
		t.Errorf("auditions = %v, want %v", got, want)
	}
}

func TestNoAuditionWhenMuted(t *testing.T) {
	if got := auditions(rootRequest(70), control.Settings{RootNote: 70, Muted: true}, nil); len(got) != 0 {
		t.Errorf("auditions = %v, want none", got)
	}
}

func TestNoAuditionForOtherRequests(t *testing.T) {
	if got := auditions(control.Request{Command: control.Volume, VolumeDB: -6}, control.Settings{VolumeDB: -6}, nil); len(got) != 0 {
		t.Errorf("auditions = %v, want none", got)
	}
}

func TestNoAuditionWhenRootChangeFails(t *testing.T) {
	if got := auditions(rootRequest(70), control.Settings{}, errors.New("no daemon")); len(got) != 0 {
		t.Errorf("auditions = %v, want none", got)
	}
}
