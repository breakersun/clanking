package transport

import (
	"bufio"
	"errors"
	"io"
	"log/slog"
	"net"
)

const lineEnd = "\n"

// ServeEvents streams event lines to each connection, starting with a snapshot, until the listener is closed.
func ServeEvents(l net.Listener, subscribe func() (<-chan string, func()), snapshot func() string) error {
	for {
		conn, err := l.Accept()
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		if err != nil {
			return err
		}
		go stream(conn, subscribe, snapshot)
	}
}

func stream(conn net.Conn, subscribe func() (<-chan string, func()), snapshot func() string) {
	lines, unsubscribe := subscribe()
	defer unsubscribe()
	err := writeLines(conn, snapshot(), lines)
	slog.Debug("events watcher left", "err", errors.Join(err, conn.Close()))
}

func writeLines(w io.Writer, first string, lines <-chan string) error {
	if _, err := io.WriteString(w, first+lineEnd); err != nil {
		return err
	}
	for line := range lines {
		if _, err := io.WriteString(w, line+lineEnd); err != nil {
			return err
		}
	}
	return nil
}

// Watch reads event lines from the daemon's events socket until the stream ends.
func Watch(path string, line func(string)) error {
	conn, err := net.Dial(streamNetwork, path)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line(scanner.Text())
	}
	return errors.Join(scanner.Err(), conn.Close())
}
