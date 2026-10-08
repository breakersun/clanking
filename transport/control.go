package transport

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/chrchr/clanking/control"
)

const (
	streamNetwork   = "unix"
	controlTimeout  = time.Second
	errorPrefix     = "error: "
	statusSeparator = "; "
)

const (
	// SpawnTimeout bounds how long a client waits for a daemon it started.
	SpawnTimeout = time.Second
	// RetryInterval is the pause between attempts to reach a starting daemon.
	RetryInterval = 20 * time.Millisecond
)

// ErrDaemon reports a request the daemon could not carry out.
var ErrDaemon = errors.New("daemon refused request")

// ListenStream binds a daemon stream socket, replacing any stale socket file.
func ListenStream(path string) (net.Listener, error) {
	if err := removeStale(path); err != nil {
		return nil, err
	}
	return net.Listen(streamNetwork, path)
}

// ServeControl answers each connection's request line with a status line until the listener is closed.
func ServeControl(l net.Listener, handle func(control.Request) (control.Settings, error), describe func() string) error {
	for {
		conn, err := l.Accept()
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		if err != nil {
			return err
		}
		go answer(conn, handle, describe)
	}
}

func answer(conn net.Conn, handle func(control.Request) (control.Settings, error), describe func() string) {
	err := errors.Join(conn.SetDeadline(time.Now().Add(controlTimeout)), reply(conn, handle, describe), conn.Close())
	if err != nil {
		slog.Warn("control connection failed", "err", err)
	}
}

func reply(conn net.Conn, handle func(control.Request) (control.Settings, error), describe func() string) error {
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return err
	}
	_, err = io.WriteString(conn, respond(line, handle, describe)+"\n")
	return err
}

func respond(line string, handle func(control.Request) (control.Settings, error), describe func() string) string {
	r, err := control.Parse(line)
	if err != nil {
		return errorPrefix + err.Error()
	}
	s, err := handle(r)
	if err != nil {
		return errorPrefix + err.Error()
	}
	return s.String() + statusSeparator + describe()
}

// Ask sends one request to the daemon and returns its status line.
func Ask(path string, r control.Request) (string, error) {
	conn, err := net.DialTimeout(streamNetwork, path, controlTimeout)
	if err != nil {
		return "", err
	}
	status, err := exchange(conn, r)
	return status, errors.Join(err, conn.Close())
}

func exchange(conn net.Conn, r control.Request) (string, error) {
	if err := conn.SetDeadline(time.Now().Add(controlTimeout)); err != nil {
		return "", err
	}
	if _, err := io.WriteString(conn, r.String()+"\n"); err != nil {
		return "", err
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return "", err
	}
	status := strings.TrimSuffix(line, "\n")
	if reason, refused := strings.CutPrefix(status, errorPrefix); refused {
		return "", fmt.Errorf("%w: %s", ErrDaemon, reason)
	}
	return status, nil
}

// Control sends a request, starting a daemon with spawn when none answers, and returns the resulting settings and grid state.
func Control(path string, r control.Request, spawn func() error) (control.Settings, string, error) {
	var status string
	ask := func() (err error) {
		status, err = Ask(path, r)
		return err
	}
	if err := Deliver(ask, spawn, SpawnTimeout, RetryInterval); err != nil {
		return control.Settings{}, "", err
	}
	settingsPart, gridState, _ := strings.Cut(status, statusSeparator)
	s, err := control.ParseSettings(settingsPart)
	return s, gridState, err
}
