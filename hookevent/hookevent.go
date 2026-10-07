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
}

type payload struct {
	Event string `json:"hook_event_name"`
	Tool  string `json:"tool_name"`
	Index int    `json:"index"`
}

// Parse reads a hook payload and returns its cue, reporting false when the event has none.
func Parse(r io.Reader) (synth.Cue, bool, error) {
	var p payload
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return "", false, err
	}
	if p.Event == preToolUse && p.Tool == askToolName {
		return synth.Ask, true, nil
	}
	if p.Event == messageDisplay && p.Index == firstDelta {
		return synth.Reply, true, nil
	}
	c, ok := eventCues[p.Event]
	return c, ok, nil
}
