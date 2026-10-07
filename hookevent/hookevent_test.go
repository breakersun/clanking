package hookevent_test

import (
	"strings"
	"testing"

	"meknik/hookevent"
	"meknik/synth"
)

type result struct {
	cue synth.Cue
	ok  bool
}

func TestParseMapsEvents(t *testing.T) {
	cases := map[string]result{
		`{"hook_event_name":"PreToolUse","tool_name":"Bash"}`:            {synth.Tick, true},
		`{"hook_event_name":"PreToolUse","tool_name":"AskUserQuestion"}`: {synth.Ask, true},
		`{"hook_event_name":"SubagentStart"}`:                            {synth.Spawn, true},
		`{"hook_event_name":"SubagentStop"}`:                             {synth.Return, true},
		`{"hook_event_name":"Stop"}`:                                     {synth.Done, true},
		`{"hook_event_name":"StopFailure"}`:                              {synth.Failed, true},
		`{"hook_event_name":"PermissionRequest","tool_name":"Bash"}`:     {synth.Permission, true},
		`{"hook_event_name":"PostToolUse","tool_name":"Bash"}`:           {"", false},
		`{"hook_event_name":"Notification"}`:                             {"", false},
		`{"hook_event_name":"MessageDisplay","index":0,"final":false}`:   {synth.Reply, true},
		`{"hook_event_name":"MessageDisplay","index":3,"final":true}`:    {"", false},
	}
	for payload, want := range cases {
		t.Run(payload, func(t *testing.T) {
			cue, ok, _ := hookevent.Parse(strings.NewReader(payload))
			if got := (result{cue, ok}); got != want {
				t.Errorf("Parse = %+v, want %+v", got, want)
			}
		})
	}
}

func TestParseRejectsMalformedJSON(t *testing.T) {
	if _, _, err := hookevent.Parse(strings.NewReader("{")); err == nil {
		t.Error("Parse error = nil, want an error")
	}
}

func TestParseRejectsEmptyInput(t *testing.T) {
	if _, _, err := hookevent.Parse(strings.NewReader("")); err == nil {
		t.Error("Parse error = nil, want an error")
	}
}
