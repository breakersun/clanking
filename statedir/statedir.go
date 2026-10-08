// Package statedir locates clanking's state files and sockets.
package statedir

import (
	"os"
	"path/filepath"
)

// ControlSocket is the name of the daemon's control socket in the run directory.
const ControlSocket = "clanking.ctl"

// CueSocket is the name of the daemon's cue socket in the run directory.
const CueSocket = "clanking.sock"

// EventsSocket is the name of the daemon's events socket in the run directory.
const EventsSocket = "clanking.events"

const (
	appName    = "clanking"
	runDirName = "run"
	logName    = "clanking.log"
	logMode    = 0o644
	dirMode    = 0o755
)

// Path returns the path of a file in clanking's state directory.
func Path(name string) (string, error) {
	dir, err := stateDir()
	return filepath.Join(dir, name), err
}

// RunPath returns the path of a file in the run directory, which containers share.
func RunPath(name string) (string, error) { return Path(filepath.Join(runDirName, name)) }

// OpenLog opens the shared log file for appending, creating the state directory.
func OpenLog() (*os.File, error) {
	path, err := Path(logName)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, logMode)
}

func stateDir() (string, error) {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, appName), nil
	}
	home, err := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", appName), err
}
