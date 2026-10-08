package main

import (
	"slices"
	"testing"
	"time"

	"meknik/control"
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
