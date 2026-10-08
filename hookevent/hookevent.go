// Package hookevent maps Claude Code hook payloads to cues.
package hookevent

import (
	"encoding/json"
	"io"

	"meknik/synth"
)

const (
	preToolUse     = "PreToolUse"
	messageDisplay = "MessageDisplay"
	askToolName    = "AskUserQuestion"
	firstDelta     = 0
)

var eventCues = map[string]synth.Cue{
	preToolUse:          synth.Tick,
	"SubagentStart":     synth.Spawn,
	"SubagentStop":      synth.Return,
	"Stop":              synth.Done,
	"StopFailure":       synth.Failed,
	"PermissionRequest": synth.Permission,
	"TaskCompleted":     synth.Milestone,
	"TaskCreated":       synth.Noted,
}

// Event is a hook's cue and the working directory of the session that raised it.
type Event struct {
	Cue synth.Cue
	Cwd string
}

type payload struct {
	Event string `json:"hook_event_name"`
	Tool  string `json:"tool_name"`
	Index int    `json:"index"`
	Cwd   string `json:"cwd"`
}

// Parse reads a hook payload and returns its event, reporting false when the event has no cue.
func Parse(r io.Reader) (Event, bool, error) {
	var p payload
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return Event{}, false, err
	}
	c, ok := cue(p)
	return Event{Cue: c, Cwd: p.Cwd}, ok, nil
}

func cue(p payload) (synth.Cue, bool) {
	if p.Event == preToolUse && p.Tool == askToolName {
		return synth.Ask, true
	}
	if p.Event == messageDisplay && p.Index == firstDelta {
		return synth.Reply, true
	}
	c, ok := eventCues[p.Event]
	return c, ok
}
