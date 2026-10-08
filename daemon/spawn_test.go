package daemon_test

import (
	"os"
	"slices"
	"testing"

	"meknik/daemon"
)

const meknikBinary = "/opt/meknik/bin/meknik"

func TestCommandSendsStderrToLog(t *testing.T) {
	logFile := os.Stdout
	if got := daemon.Command(meknikBinary, logFile).Stderr; got != logFile {
		t.Errorf("Stderr = %v, want %v", got, logFile)
	}
}

func TestCommandRunsGivenBinaryAsDaemon(t *testing.T) {
	if got := daemon.Command(meknikBinary, os.Stdout).Args; !slices.Equal(got, []string{meknikBinary, "daemon"}) {
		t.Errorf("Args = %q, want [%s daemon]", got, meknikBinary)
	}
}

func TestSpawnReportsMissingBinary(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := daemon.Spawn("/nonexistent/meknik"); err == nil {
		t.Error("Spawn error = nil, want an error")
	}
}
