package statedir_test

import (
	"testing"

	"meknik/statedir"
)

func TestPathLivesUnderStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/state")
	if got, _ := statedir.Path("meknik.log"); got != "/state/meknik/meknik.log" {
		t.Errorf("Path = %q, want /state/meknik/meknik.log", got)
	}
}

func TestPathFallsBackToHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/u")
	if got, _ := statedir.Path("meknik.log"); got != "/home/u/.local/state/meknik/meknik.log" {
		t.Errorf("Path = %q, want /home/u/.local/state/meknik/meknik.log", got)
	}
}

func TestRunPathLivesInSharedRunDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/state")
	if got, _ := statedir.RunPath(statedir.ControlSocket); got != "/state/meknik/run/meknik.ctl" {
		t.Errorf("RunPath = %q, want /state/meknik/run/meknik.ctl", got)
	}
}

func TestOpenLogCreatesStateDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	f, err := statedir.OpenLog()
	if err == nil {
		_ = f.Close()
	}
	if err != nil {
		t.Errorf("OpenLog error = %v, want nil", err)
	}
}

func TestCueSocketLivesInSharedRunDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/state")
	if got, _ := statedir.RunPath(statedir.CueSocket); got != "/state/meknik/run/meknik.sock" {
		t.Errorf("RunPath = %q, want /state/meknik/run/meknik.sock", got)
	}
}

func TestEventsSocketLivesInSharedRunDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/state")
	if got, _ := statedir.RunPath(statedir.EventsSocket); got != "/state/meknik/run/meknik.events" {
		t.Errorf("RunPath = %q, want /state/meknik/run/meknik.events", got)
	}
}
