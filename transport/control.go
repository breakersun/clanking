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

	"meknik/control"
)

const (
	controlNetwork = "unix"
	controlTimeout = time.Second
	errorPrefix    = "error: "
)

const (
	// SpawnTimeout bounds how long a client waits for a daemon it started.
	SpawnTimeout = time.Second
	// RetryInterval is the pause between attempts to reach a starting daemon.
	RetryInterval = 20 * time.Millisecond
)

// ErrDaemon reports a request the daemon could not carry out.
var ErrDaemon = errors.New("daemon refused request")

// ListenControl binds the daemon's control socket, replacing any stale socket file.
func ListenControl(path string) (net.Listener, error) {
	if err := removeStale(path); err != nil {
		return nil, err
	}
	return net.Listen(controlNetwork, path)
}

// ServeControl answers each connection's request line with a status line until the listener is closed.
func ServeControl(l net.Listener, handle func(control.Request) (control.Settings, error)) error {
	for {
		conn, err := l.Accept()
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		if err != nil {
			return err
		}
		go answer(conn, handle)
	}
}

func answer(conn net.Conn, handle func(control.Request) (control.Settings, error)) {
	err := errors.Join(conn.SetDeadline(time.Now().Add(controlTimeout)), reply(conn, handle), conn.Close())
	if err != nil {
		slog.Warn("control connection failed", "err", err)
	}
}

func reply(conn net.Conn, handle func(control.Request) (control.Settings, error)) error {
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return err
	}
	_, err = io.WriteString(conn, respond(line, handle)+"\n")
	return err
}

func respond(line string, handle func(control.Request) (control.Settings, error)) string {
	r, err := control.Parse(line)
	if err != nil {
		return errorPrefix + err.Error()
	}
	s, err := handle(r)
	if err != nil {
		return errorPrefix + err.Error()
	}
	return s.String()
}

// Ask sends one request to the daemon and returns its status line.
func Ask(path string, r control.Request) (string, error) {
	conn, err := net.DialTimeout(controlNetwork, path, controlTimeout)
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

// Control sends a request, starting a daemon with spawn when none answers, and returns the resulting settings.
func Control(path string, r control.Request, spawn func() error) (control.Settings, error) {
	var status string
	ask := func() (err error) {
		status, err = Ask(path, r)
		return err
	}
	if err := Deliver(ask, spawn, SpawnTimeout, RetryInterval); err != nil {
		return control.Settings{}, err
	}
	return control.ParseSettings(status)
}
