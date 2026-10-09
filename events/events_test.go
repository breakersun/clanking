package events_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/chrchr/clanking/control"
	"github.com/chrchr/clanking/events"
	"github.com/chrchr/clanking/synth"
)

const subscriberQueue = 64

func drain(lines <-chan string) []string {
	var got []string
	for {
		select {
		case line := <-lines:
			got = append(got, line)
		default:
			return got
		}
	}
}

func ignoreCue(synth.Cue)             {}
func ignoreSettings(control.Settings) {}

func TestSubscriberGetsPublishedLinesInOrder(t *testing.T) {
	hub := events.NewHub()
	lines, _ := hub.Subscribe()
	hub.Publish("cue tick")
	hub.Publish("cue done")
	if got, want := drain(lines), []string{"cue tick", "cue done"}; !slices.Equal(got, want) {
		t.Errorf("lines = %q, want %q", got, want)
	}
}

func TestPublishDoesNotBlockOnFullSubscriber(t *testing.T) {
	hub := events.NewHub()
	lines, _ := hub.Subscribe()
	for range subscriberQueue + 1 {
		hub.Publish("cue tick")
	}
	if got := len(drain(lines)); got != subscriberQueue {
		t.Errorf("buffered = %d, want %d", got, subscriberQueue)
	}
}

func TestUnsubscribedChannelGetsNothing(t *testing.T) {
	hub := events.NewHub()
	lines, unsubscribe := hub.Subscribe()
	unsubscribe()
	hub.Publish("cue tick")
	if line, open := <-lines; open {
		t.Errorf("received %q after unsubscribe", line)
	}
}

func TestCueLine(t *testing.T) {
	if got := events.CueLine(synth.Spawn); got != "cue spawn" {
		t.Errorf("CueLine = %q, want %q", got, "cue spawn")
	}
}

func TestSettingsLine(t *testing.T) {
	if got := events.SettingsLine(control.Settings{VolumeDB: -6, Muted: true, TempoBPM: 120, RootNote: 74}); got != "settings volume -6 dB, muted, root D5, 120 bpm" {
		t.Errorf("SettingsLine = %q, want %q", got, "settings volume -6 dB, muted, root D5, 120 bpm")
	}
}

func TestDispatchCallsCueForCueLine(t *testing.T) {
	var got synth.Cue
	_ = events.Dispatch(events.CueLine(synth.Ask), func(c synth.Cue) { got = c }, ignoreSettings)
	if got != synth.Ask {
		t.Errorf("cue = %q, want %q", got, synth.Ask)
	}
}

func TestDispatchCallsSettingsForSettingsLine(t *testing.T) {
	want := control.Settings{VolumeDB: -12.5, TempoBPM: 120}
	var got control.Settings
	_ = events.Dispatch(events.SettingsLine(want), ignoreCue, func(s control.Settings) { got = s })
	if got != want {
		t.Errorf("settings = %+v, want %+v", got, want)
	}
}

func TestDispatchRejectsBadLines(t *testing.T) {
	for _, line := range []string{"", "volume -6 dB, muted", "settings loud"} {
		t.Run(line, func(t *testing.T) {
			if err := events.Dispatch(line, ignoreCue, ignoreSettings); !errors.Is(err, events.ErrLine) {
				t.Errorf("Dispatch(%q) error = %v, want %v", line, err, events.ErrLine)
			}
		})
	}
}
