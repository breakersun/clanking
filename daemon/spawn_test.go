package daemon_test

import (
	"os"
	"slices"
	"testing"

	"github.com/chrchr/clanking/daemon"
)

const clankingBinary = "/opt/clanking/bin/clanking"

func TestCommandSendsStderrToLog(t *testing.T) {
	logFile := os.Stdout
	if got := daemon.Command(clankingBinary, logFile).Stderr; got != logFile {
		t.Errorf("Stderr = %v, want %v", got, logFile)
	}
}

func TestCommandRunsGivenBinaryAsDaemon(t *testing.T) {
	if got := daemon.Command(clankingBinary, os.Stdout).Args; !slices.Equal(got, []string{clankingBinary, "daemon"}) {
		t.Errorf("Args = %q, want [%s daemon]", got, clankingBinary)
	}
}

func TestSpawnReportsMissingBinary(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := daemon.Spawn("/nonexistent/clanking"); err == nil {
		t.Error("Spawn error = nil, want an error")
	}
}
