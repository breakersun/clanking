package transport_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"meknik/synth"
	"meknik/transport"
)

const (
	socketName    = "meknik.sock"
	receiveWait   = time.Second
	shortTimeout  = 20 * time.Millisecond
	retryInterval = 5 * time.Millisecond
	stallWait     = 2 * time.Second
)

var errSpawn = errors.New("spawn failed")

func socketPath(t *testing.T) string { return filepath.Join(t.TempDir(), socketName) }

func serve(t *testing.T, path string) <-chan synth.Cue {
	t.Helper()
	conn, err := transport.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	cues := make(chan synth.Cue, 1)
	go func() { _ = transport.Serve(conn, func(c synth.Cue) { cues <- c }) }()
	return cues
}

func receive(cues <-chan synth.Cue) synth.Cue {
	select {
	case c := <-cues:
		return c
	case <-time.After(receiveWait):
		return ""
	}
}

func noSpawn() error { return nil }

func TestSendReachesServer(t *testing.T) {
	path := socketPath(t)
	cues := serve(t, path)
	_ = transport.Send(path, synth.Done)
	if got := receive(cues); got != synth.Done {
		t.Errorf("received %q, want %q", got, synth.Done)
	}
}

func TestSendWithoutDaemonFails(t *testing.T) {
	if err := transport.Send(socketPath(t), synth.Done); err == nil {
		t.Error("Send error = nil, want an error")
	}
}

func TestListenReplacesStaleSocket(t *testing.T) {
	path := socketPath(t)
	_ = os.WriteFile(path, nil, 0o600)
	cues := serve(t, path)
	_ = transport.Send(path, synth.Tick)
	if got := receive(cues); got != synth.Tick {
		t.Errorf("received %q, want %q", got, synth.Tick)
	}
}

func TestServeReturnsNilWhenClosed(t *testing.T) {
	conn, _ := transport.Listen(socketPath(t))
	_ = conn.Close()
	if err := transport.Serve(conn, func(synth.Cue) {}); err != nil {
		t.Errorf("Serve error = %v, want nil", err)
	}
}

func TestDeliverSkipsSpawnWhenDaemonRuns(t *testing.T) {
	path := socketPath(t)
	serve(t, path)
	spawned := false
	_ = transport.Deliver(path, synth.Tick, func() error { spawned = true; return nil }, shortTimeout, retryInterval)
	if spawned {
		t.Error("spawned = true, want false")
	}
}

func TestDeliverSpawnsDaemonAndSends(t *testing.T) {
	path := socketPath(t)
	var cues <-chan synth.Cue
	_ = transport.Deliver(path, synth.Done, func() error { cues = serve(t, path); return nil }, shortTimeout, retryInterval)
	if got := receive(cues); got != synth.Done {
		t.Errorf("received %q, want %q", got, synth.Done)
	}
}

func TestDeliverReportsSpawnFailure(t *testing.T) {
	err := transport.Deliver(socketPath(t), synth.Done, func() error { return errSpawn }, shortTimeout, retryInterval)
	if !errors.Is(err, errSpawn) {
		t.Errorf("Deliver error = %v, want %v", err, errSpawn)
	}
}

func TestDeliverTimesOutWhenDaemonNeverAppears(t *testing.T) {
	if err := transport.Deliver(socketPath(t), synth.Done, noSpawn, shortTimeout, retryInterval); err == nil {
		t.Error("Deliver error = nil, want an error")
	}
}

func TestSendGivesUpWhenDaemonStopsReading(t *testing.T) {
	path := socketPath(t)
	conn, err := transport.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	failed := make(chan struct{})
	go func() {
		var err error
		for err == nil {
			err = transport.Send(path, synth.Tick)
		}
		close(failed)
	}()
	select {
	case <-failed:
	case <-time.After(stallWait):
		t.Error("Send still blocked after the daemon stopped reading")
	}
}
