// Package events fans daemon event lines out to watchers and reads them back.
package events

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"meknik/control"
	"meknik/synth"
)

const (
	subscriberQueue = 64
	cueKind         = "cue"
	settingsKind    = "settings"
	kindSeparator   = " "
)

// ErrLine reports an event line that Dispatch cannot read.
var ErrLine = errors.New("invalid event line")

// Hub fans published lines out to every subscriber without blocking.
type Hub struct {
	mu          sync.Mutex
	subscribers map[chan string]struct{}
}

// NewHub returns a hub with no subscribers.
func NewHub() *Hub { return &Hub{subscribers: map[chan string]struct{}{}} }

// Subscribe returns a buffered channel of published lines and a function that ends the subscription.
func (h *Hub) Subscribe() (<-chan string, func()) {
	lines := make(chan string, subscriberQueue)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subscribers[lines] = struct{}{}
	return lines, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.subscribers, lines)
		close(lines)
	}
}

// Publish sends a line to every subscriber, dropping it for any whose buffer is full.
func (h *Hub) Publish(line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for lines := range h.subscribers {
		select {
		case lines <- line:
		default:
		}
	}
}

// CueLine is the event line for a cue the daemon accepted.
func CueLine(c synth.Cue) string { return cueKind + kindSeparator + string(c) }

// SettingsLine is the event line for the daemon's current settings.
func SettingsLine(s control.Settings) string { return settingsKind + kindSeparator + s.String() }

// Dispatch reads one event line and calls the function for its kind.
func Dispatch(line string, cue func(synth.Cue), settings func(control.Settings)) error {
	kind, rest, _ := strings.Cut(line, kindSeparator)
	switch kind {
	case cueKind:
		cue(synth.Cue(rest))
		return nil
	case settingsKind:
		s, err := control.ParseSettings(rest)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrLine, err)
		}
		settings(s)
		return nil
	}
	return fmt.Errorf("%w: %q", ErrLine, line)
}
