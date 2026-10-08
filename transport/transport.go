// Package transport carries cue names from hook clients to the daemon over a Unix datagram socket.
package transport

import (
	"errors"
	"io/fs"
	"net"
	"os"
	"time"

	"meknik/synth"
)

const (
	network     = "unixgram"
	maxDatagram = 64
	sendTimeout = 100 * time.Millisecond
)

// Listen binds the daemon's cue socket, replacing any stale socket file.
func Listen(path string) (*net.UnixConn, error) {
	if err := removeStale(path); err != nil {
		return nil, err
	}
	return net.ListenUnixgram(network, &net.UnixAddr{Name: path, Net: network})
}

func removeStale(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Serve hands each received cue to play until the connection is closed.
func Serve(conn *net.UnixConn, play func(synth.Cue)) error {
	buf := make([]byte, maxDatagram)
	for {
		n, err := conn.Read(buf)
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		if err != nil {
			return err
		}
		play(synth.Cue(buf[:n]))
	}
}

// Send delivers one cue to the daemon socket.
func Send(path string, c synth.Cue) error {
	conn, err := net.Dial(network, path)
	if err != nil {
		return err
	}
	if err := conn.SetWriteDeadline(time.Now().Add(sendTimeout)); err != nil {
		return errors.Join(err, conn.Close())
	}
	_, err = conn.Write([]byte(c))
	return errors.Join(err, conn.Close())
}

// Deliver makes one attempt to reach the daemon; when that fails it spawns one and retries until timeout.
func Deliver(attempt, spawn func() error, timeout, interval time.Duration) error {
	if attempt() == nil {
		return nil
	}
	if err := spawn(); err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for {
		time.Sleep(interval)
		err := attempt()
		if err == nil || time.Now().After(deadline) {
			return err
		}
	}
}
