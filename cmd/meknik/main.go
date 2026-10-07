// Command meknik plays synthesized audio cues for Claude Code hook events.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"syscall"
	"time"

	"meknik/daemon"
	"meknik/hookevent"
	"meknik/mixer"
	"meknik/playback"
	"meknik/synth"
	"meknik/transport"
)

const (
	appName          = "meknik"
	socketName       = "meknik.sock"
	lockName         = "meknik.lock"
	logName          = "meknik.log"
	runDirName       = "run"
	noSpawnFlag      = "--no-spawn"
	gainEnv          = "MEKNIK_GAIN"
	idleEnv          = "MEKNIK_IDLE_TIMEOUT"
	spawnTimeout     = time.Second
	retryInterval    = 20 * time.Millisecond
	limiterCeilingDB = -24.0
	soundQueue       = 64
	logMode          = 0o644
	dirMode          = 0o755
)

var (
	errUsage         = errors.New("usage: meknik hook [--no-spawn] | meknik daemon | meknik play <cue>")
	errSpawnDisabled = errors.New("no daemon is running and spawning is disabled")
)

func main() {
	logFile, err := openLog()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, nil)))
	if err := run(os.Args[1:]); err != nil {
		slog.Error("meknik failed", "args", os.Args[1:], "err", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	switch {
	case slices.Equal(args, []string{"hook"}):
		return runHook(spawnDaemon)
	case slices.Equal(args, []string{"hook", noSpawnFlag}):
		return runHook(refuseSpawn)
	case slices.Equal(args, []string{"daemon"}):
		return runDaemon()
	case len(args) == 2 && args[0] == "play":
		return runPlay(synth.Cue(args[1]))
	}
	return errUsage
}

func runHook(spawn func() error) error {
	cue, ok, err := hookevent.Parse(os.Stdin)
	if err != nil || !ok {
		return err
	}
	socket, err := runPath(socketName)
	if err != nil {
		return err
	}
	return transport.Deliver(socket, cue, spawn, spawnTimeout, retryInterval)
}

func refuseSpawn() error { return errSpawnDisabled }

func spawnDaemon() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := openLog()
	if err != nil {
		return err
	}
	cmd := daemonCommand(self, logFile)
	if err := cmd.Start(); err != nil {
		return errors.Join(err, logFile.Close())
	}
	return errors.Join(cmd.Process.Release(), logFile.Close())
}

func daemonCommand(self string, logFile *os.File) *exec.Cmd {
	cmd := exec.Command(self, "daemon")
	cmd.Dir = "/"
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}

func runDaemon() error {
	lockPath, err := runPath(lockName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), dirMode); err != nil {
		return err
	}
	lock, held, err := daemon.Lock(lockPath)
	if err != nil || !held {
		return err
	}
	defer func() { _ = lock.Close() }()
	gain, err := parseGain(os.Getenv(gainEnv))
	if err != nil {
		return err
	}
	idle, err := parseIdle(os.Getenv(idleEnv))
	if err != nil {
		return err
	}
	socket, err := runPath(socketName)
	if err != nil {
		return err
	}
	conn, err := transport.Listen(socket)
	if err != nil {
		return err
	}
	stream, err := playback.Start(playback.Command())
	if err != nil {
		return errors.Join(err, conn.Close(), os.Remove(socket))
	}
	sounds := make(chan mixer.Sound, soundQueue)
	go func() {
		if err := transport.Serve(conn, enqueue(sounds, newRNG())); err != nil {
			slog.Error("socket failed", "err", err)
		}
	}()
	slog.Info("daemon started", "socket", socket, "gain", gain, "idle", idle)
	playErr := daemon.Play(mixer.New(gain, synth.Amplitude(limiterCeilingDB)), sounds, stream, idle, time.Now)
	slog.Info("daemon stopping")
	return errors.Join(playErr, conn.Close(), os.Remove(socket), stream.Close())
}

func enqueue(sounds chan<- mixer.Sound, rng *rand.Rand) func(synth.Cue) {
	return func(c synth.Cue) {
		samples, ok := synth.Sound(c, rng)
		if !ok {
			slog.Warn("unknown cue", "cue", c)
			return
		}
		select {
		case sounds <- mixer.Sound{Samples: samples, Tick: c == synth.Tick}:
		default:
			slog.Warn("sound queue full, cue dropped", "cue", c)
		}
	}
}

func runPlay(c synth.Cue) error {
	samples, ok := synth.Sound(c, newRNG())
	if !ok {
		return fmt.Errorf("unknown cue %q", c)
	}
	stream, err := playback.Start(playback.Command())
	if err != nil {
		return err
	}
	_, err = stream.Write(playback.Encode(nil, samples))
	return errors.Join(err, stream.Close())
}

func parseGain(db string) (float64, error) {
	if db == "" {
		return 1, nil
	}
	v, err := strconv.ParseFloat(db, 64)
	return synth.Amplitude(v), err
}

func parseIdle(timeout string) (time.Duration, error) {
	if timeout == "" {
		return daemon.NeverIdle, nil
	}
	return time.ParseDuration(timeout)
}

func newRNG() *rand.Rand { return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())) }

func runPath(name string) (string, error) { return statePath(filepath.Join(runDirName, name)) }

func statePath(name string) (string, error) {
	dir, err := stateDir()
	return filepath.Join(dir, name), err
}

func openLog() (*os.File, error) {
	path, err := statePath(logName)
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
