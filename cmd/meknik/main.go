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
	"strings"
	"syscall"
	"time"

	"meknik/control"
	"meknik/daemon"
	"meknik/hookevent"
	"meknik/mixer"
	"meknik/playback"
	"meknik/statedir"
	"meknik/synth"
	"meknik/transport"
)

const (
	socketName       = "meknik.sock"
	lockName         = "meknik.lock"
	settingsName     = "mixer.json"
	noSpawnFlag      = "--no-spawn"
	idleEnv          = "MEKNIK_IDLE_TIMEOUT"
	spawnTimeout     = time.Second
	retryInterval    = 20 * time.Millisecond
	limiterCeilingDB = -24.0
	soundQueue       = 64
	dirMode          = 0o755
)

var (
	errUsage         = errors.New("usage: meknik hook [--no-spawn] | daemon | play <cue> | mute | unmute | toggle | volume <dB> | status")
	errSpawnDisabled = errors.New("no daemon is running and spawning is disabled")
)

func main() {
	logFile, err := statedir.OpenLog()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, nil)))
	if err := run(os.Args[1:]); err != nil {
		slog.Error("meknik failed", "args", os.Args[1:], "err", err)
		fmt.Fprintln(os.Stderr, "meknik:", err)
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
	case len(args) == 1 && slices.Contains([]string{control.Mute, control.Unmute, control.Toggle, control.Status}, args[0]):
		return runControl(args[0])
	case len(args) == 2 && args[0] == control.Volume:
		return runControl(strings.Join(args, " "))
	}
	return errUsage
}

func runHook(spawn func() error) error {
	cue, ok, err := hookevent.Parse(os.Stdin)
	if err != nil || !ok {
		return err
	}
	socket, err := statedir.RunPath(socketName)
	if err != nil {
		return err
	}
	return transport.Deliver(func() error { return transport.Send(socket, cue) }, spawn, spawnTimeout, retryInterval)
}

func runControl(line string) error {
	r, err := control.Parse(line)
	if err != nil {
		return err
	}
	path, err := statedir.RunPath(statedir.ControlSocket)
	if err != nil {
		return err
	}
	var status string
	ask := func() (err error) {
		status, err = transport.Ask(path, r)
		return err
	}
	if err := transport.Deliver(ask, spawnDaemon, spawnTimeout, retryInterval); err != nil {
		return err
	}
	_, err = fmt.Println(status)
	return err
}

func refuseSpawn() error { return errSpawnDisabled }

func spawnDaemon() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := statedir.OpenLog()
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
	lockPath, err := statedir.RunPath(lockName)
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
	idle, err := parseIdle(os.Getenv(idleEnv))
	if err != nil {
		return err
	}
	settingsPath, err := statedir.Path(settingsName)
	if err != nil {
		return err
	}
	settings, err := control.Load(settingsPath)
	if err != nil {
		return err
	}
	socket, err := statedir.RunPath(socketName)
	if err != nil {
		return err
	}
	controlSocket, err := statedir.RunPath(statedir.ControlSocket)
	if err != nil {
		return err
	}
	conn, err := transport.Listen(socket)
	if err != nil {
		return err
	}
	listener, err := transport.ListenControl(controlSocket)
	if err != nil {
		return errors.Join(err, conn.Close(), os.Remove(socket))
	}
	stream, err := playback.Start(playback.Command())
	if err != nil {
		return errors.Join(err, conn.Close(), os.Remove(socket), listener.Close())
	}
	sounds := make(chan mixer.Sound, soundQueue)
	gains := make(chan float64, soundQueue)
	controller := control.NewController(settings,
		func(s control.Settings) error { return control.Save(settingsPath, s) },
		func(s control.Settings) { gains <- s.Gain() })
	go func() {
		if err := transport.Serve(conn, enqueue(sounds, newRNG())); err != nil {
			slog.Error("cue socket failed", "err", err)
		}
	}()
	go func() {
		if err := transport.ServeControl(listener, controller.Handle); err != nil {
			slog.Error("control socket failed", "err", err)
		}
	}()
	slog.Info("daemon started", "socket", socket, "settings", settings.String(), "idle", idle)
	playErr := daemon.Play(mixer.New(settings.Gain(), synth.Amplitude(limiterCeilingDB)), sounds, gains, stream, idle, time.Now)
	slog.Info("daemon stopping")
	return errors.Join(playErr, conn.Close(), os.Remove(socket), listener.Close(), stream.Close())
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

func parseIdle(timeout string) (time.Duration, error) {
	if timeout == "" {
		return daemon.NeverIdle, nil
	}
	return time.ParseDuration(timeout)
}

func newRNG() *rand.Rand { return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())) }
