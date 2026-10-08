package transport_test

import (
	"errors"
	"path/filepath"
	"testing"

	"meknik/control"
	"meknik/transport"
)

const controlName = "meknik.ctl"

var errSave = errors.New("save failed")

func controlPath(t *testing.T) string { return filepath.Join(t.TempDir(), controlName) }

func serveControl(t *testing.T, path string, handle func(control.Request) (control.Settings, error)) {
	t.Helper()
	l, err := transport.ListenControl(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() { _ = transport.ServeControl(l, handle) }()
}

func TestAskReturnsDaemonStatus(t *testing.T) {
	path := controlPath(t)
	serveControl(t, path, func(r control.Request) (control.Settings, error) {
		return control.Settings{}.Apply(r), nil
	})
	got, _ := transport.Ask(path, control.Request{Command: control.Volume, VolumeDB: -6})
	if want := "volume -6 dB, unmuted"; got != want {
		t.Errorf("Ask = %q, want %q", got, want)
	}
}

func TestAskReportsDaemonError(t *testing.T) {
	path := controlPath(t)
	serveControl(t, path, func(control.Request) (control.Settings, error) { return control.Settings{}, errSave })
	if _, err := transport.Ask(path, control.Request{Command: control.Mute}); !errors.Is(err, transport.ErrDaemon) {
		t.Errorf("Ask error = %v, want %v", err, transport.ErrDaemon)
	}
}

func TestAskWithoutDaemonFails(t *testing.T) {
	if _, err := transport.Ask(controlPath(t), control.Request{Command: control.Status}); err == nil {
		t.Error("Ask error = nil, want an error")
	}
}

func TestServeControlReturnsNilWhenClosed(t *testing.T) {
	l, _ := transport.ListenControl(controlPath(t))
	_ = l.Close()
	if err := transport.ServeControl(l, nil); err != nil {
		t.Errorf("ServeControl error = %v, want nil", err)
	}
}
