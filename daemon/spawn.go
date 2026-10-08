package daemon

import (
	"errors"
	"os"
	"os/exec"
	"syscall"

	"meknik/statedir"
)

// Command returns the detached `meknik daemon` process, logging its stderr to logFile.
func Command(binary string, logFile *os.File) *exec.Cmd {
	cmd := exec.Command(binary, "daemon")
	cmd.Dir = "/"
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}

// Spawn starts a detached daemon from binary.
func Spawn(binary string) error {
	logFile, err := statedir.OpenLog()
	if err != nil {
		return err
	}
	cmd := Command(binary, logFile)
	if err := cmd.Start(); err != nil {
		return errors.Join(err, logFile.Close())
	}
	return errors.Join(cmd.Process.Release(), logFile.Close())
}
