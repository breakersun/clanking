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

// Listen binds the daemon socket, replacing any stale socket file.
func Listen(path string) (*net.UnixConn, error) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return net.ListenUnixgram(network, &net.UnixAddr{Name: path, Net: network})
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

// Deliver sends a cue; when no daemon answers it spawns one and retries until timeout.
func Deliver(path string, c synth.Cue, spawn func() error, timeout, interval time.Duration) error {
	if Send(path, c) == nil {
		return nil
	}
	if err := spawn(); err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for {
		time.Sleep(interval)
		err := Send(path, c)
		if err == nil || time.Now().After(deadline) {
			return err
		}
	}
}
