package synth_test

import (
	"slices"
	"testing"

	"github.com/chrchr/clanking/synth"
)

var xdgSounds = []synth.Cue{
	"dialog-warning", "bell", "alarm-clock-elapsed", "trash-empty", "service-login", "service-logout",
	"suspend-error", "phone-incoming-call", "phone-outgoing-calling", "phone-outgoing-busy", "audio-volume-change",
	"camera-shutter", "screen-capture", "network-connectivity-established", "network-connectivity-lost",
	"power-plug", "power-unplug", "battery-low",
}

var xdgAliases = map[synth.Cue]synth.Cue{
	"complete": synth.Done, "dialog-error": synth.Failed, "dialog-question": synth.Ask,
	"dialog-information": synth.Noted, "window-attention": synth.Permission, "message": synth.Reply,
	"message-new-instant": synth.Reply, "device-added": synth.Spawn, "device-removed": synth.Return,
}

func TestThemeCuesListsTheXDGSounds(t *testing.T) {
	if !slices.Equal(synth.ThemeCues, xdgSounds) {
		t.Errorf("ThemeCues = %v, want %v", synth.ThemeCues, xdgSounds)
	}
}

func TestEveryThemeCueHasASound(t *testing.T) {
	for _, c := range synth.ThemeCues {
		t.Run(string(c), func(t *testing.T) {
			if _, ok := synth.Sound(c, testRoot, newRNG()); !ok {
				t.Errorf("Sound(%q) unknown", c)
			}
		})
	}
}

func TestThemeCuesStartSilent(t *testing.T) {
	for _, c := range synth.ThemeCues {
		t.Run(string(c), func(t *testing.T) {
			if got := sound(t, c)[0]; got != 0 {
				t.Errorf("first sample = %v, want 0", got)
			}
		})
	}
}

func TestThemeCuesEndSilent(t *testing.T) {
	for _, c := range synth.ThemeCues {
		t.Run(string(c), func(t *testing.T) {
			samples := sound(t, c)
			if got := samples[len(samples)-1]; got != 0 {
				t.Errorf("last sample = %v, want 0", got)
			}
		})
	}
}

func TestAliasesResolveToHookCues(t *testing.T) {
	for alias, want := range xdgAliases {
		t.Run(string(alias), func(t *testing.T) {
			if got := synth.Canonical(alias); got != want {
				t.Errorf("Canonical(%q) = %q, want %q", alias, got, want)
			}
		})
	}
}

func TestCanonicalKeepsAnUnaliasedCue(t *testing.T) {
	if got := synth.Canonical(synth.Done); got != synth.Done {
		t.Errorf("Canonical(done) = %q, want done", got)
	}
}

func TestAliasesListsEveryAliasSorted(t *testing.T) {
	want := []synth.Cue{"complete", "device-added", "device-removed", "dialog-error", "dialog-information", "dialog-question", "message", "message-new-instant", "window-attention"}
	if got := synth.Aliases(); !slices.Equal(got, want) {
		t.Errorf("Aliases() = %v, want %v", got, want)
	}
}

func TestKnownAcceptsEveryName(t *testing.T) {
	var unknown []synth.Cue
	for _, c := range slices.Concat(synth.Cues, synth.UICues, synth.ThemeCues, synth.Aliases()) {
		if !synth.Known(c) {
			unknown = append(unknown, c)
		}
	}
	if len(unknown) != 0 {
		t.Errorf("unknown names = %v", unknown)
	}
}

func TestKnownRejectsBogus(t *testing.T) {
	if synth.Known("bogus") {
		t.Error("Known(bogus) = true, want false")
	}
}
