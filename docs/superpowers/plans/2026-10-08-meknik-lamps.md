# meknik lamps and events Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The daemon streams cue and settings events on a new socket; `meknik-ui` shows a row of per-cue lamps that flash and fade, follows every settings change, and gets an inner margin.

**Architecture:** A GTK-free `events` package fans event lines out to subscribers and formats/parses them. `transport` gains a stream server and a watcher for the events socket. The daemon publishes a `cue` line when it accepts a cue and a `settings` line when settings change, and sends a settings snapshot to each new watcher. The window watches the stream, lights lamps with a CSS transition, and routes settings through its existing `show`.

**Tech Stack:** Go 1.26 in the podman build image, gotk4 v0.3.1 (GTK 4.18), Fossil.

**Spec:** `docs/superpowers/specs/2026-10-07-meknik-lamps-design.md`

## Global Constraints

- Events socket: `$XDG_STATE_HOME/meknik/run/meknik.events` = `statedir.RunPath(statedir.EventsSocket)`, `EventsSocket = "meknik.events"`.
- Event lines: `cue <name>` and `settings <Settings.String()>`, newline-terminated. First line on every connection is a `settings` snapshot.
- Cue events are published only for cues accepted into the mixer, muted or not. Dropped or unknown cues publish nothing.
- `Hub.Publish` never blocks; a full subscriber buffer (`subscriberQueue` = 64) drops that line for that subscriber only.
- `transport.ListenControl` is renamed `transport.ListenStream`.
- Window: `windowMargin` 12 px on all sides; row spacing stays 6; lamps above the volume scale, in `synth.Cues` order, coloured, cue-name tooltips; fade `200ms ease-out`; `reconnectInterval` 1 s.
- Settings events update the window through `show` (the `syncing` guard), never sending a request.
- Test: `make test`. Lint: `make lint`. Both must pass before each commit.
- Fossil commits: one line, no trailers, each path listed; `fossil add` new files first. Never git; never `fossil revert`/`fossil rm`.
- Code style per `~/.claude/CLAUDE.md`: no comments beyond one-line doc comments and package headers; one assertion per test; tests < 5 s; named constants; no defensive checks; no fallbacks; DRY; `slog` for logging.

## Review Focus

1. **A watcher disconnects** (window closed): its goroutine must end at the next write and unsubscribe, so the hub does not keep feeding a dead channel. Pinned by `TestUnsubscribedChannelGetsNothing` (Task 1); the write-error exit is checked by the reviewer of Task 2.
2. **Settings change between subscribe and snapshot**: `ServeEvents` subscribes before it takes the snapshot, so a change in between arrives as a later line, never lost. Checked by the Task 2 reviewer.
3. **Daemon not yet running when the window opens**: the watcher's first `Watch` fails; it logs, waits 1 s, and connects once the status request has started the daemon. Checked by hand in Task 4.
4. **Burst of ticks**: many cue events in a row must not stall audio or the window; `Publish` drops on a full buffer (pinned by `TestPublishDoesNotBlockOnFullSubscriber`, Task 1), and each lamp keeps one pending un-light timer. Checked by hand in Task 4.
5. **Settings event while the user drags the scale**: `show` moves the scale under the `syncing` guard and sends nothing; a pending debounced volume send may then send the reply's value (known, accepted earlier). Checked by the Task 4 reviewer.

---

## File Structure

| Path | Responsibility |
|---|---|
| `events/events.go`, `events/events_test.go` | `Hub`, `CueLine`, `SettingsLine`, `Dispatch`, `ErrLine`. New. |
| `statedir/statedir.go`, `statedir/statedir_test.go` | Gains `EventsSocket`. Modified. |
| `transport/control.go`, `transport/control_test.go` | `ListenControl` → `ListenStream`. Modified. |
| `transport/events.go`, `transport/events_test.go` | `ServeEvents`, `Watch`. New. |
| `cmd/meknik/main.go`, `cmd/meknik/main_test.go` | Daemon publishes events and serves the events socket. Modified. |
| `cmd/meknik-ui/lamps.go`, `cmd/meknik-ui/lamps_test.go` | Lamp colours, CSS, lamp row. New. |
| `cmd/meknik-ui/main.go` | Margin, lamp row, event watcher. Modified. |

---

### Task 1: `events` package

**Files:**
- Create: `events/events.go`, `events/events_test.go`

**Interfaces:**
- Consumes: `synth.Cue`, `control.Settings`, `control.Settings.String`, `control.ParseSettings`.
- Produces:
  - `func events.NewHub() *events.Hub`
  - `func (h *Hub) Subscribe() (<-chan string, func())`
  - `func (h *Hub) Publish(line string)`
  - `func events.CueLine(c synth.Cue) string`
  - `func events.SettingsLine(s control.Settings) string`
  - `func events.Dispatch(line string, cue func(synth.Cue), settings func(control.Settings)) error`
  - `var events.ErrLine = errors.New("invalid event line")`

- [ ] **Step 1: Write the failing tests** — `events/events_test.go`

```go
package events_test

import (
	"errors"
	"slices"
	"testing"

	"meknik/control"
	"meknik/events"
	"meknik/synth"
)

const subscriberQueue = 64

func drain(lines <-chan string) []string {
	var got []string
	for {
		select {
		case line := <-lines:
			got = append(got, line)
		default:
			return got
		}
	}
}

func ignoreCue(synth.Cue)                {}
func ignoreSettings(control.Settings) {}

func TestSubscriberGetsPublishedLinesInOrder(t *testing.T) {
	hub := events.NewHub()
	lines, _ := hub.Subscribe()
	hub.Publish("cue tick")
	hub.Publish("cue done")
	if got, want := drain(lines), []string{"cue tick", "cue done"}; !slices.Equal(got, want) {
		t.Errorf("lines = %q, want %q", got, want)
	}
}

func TestPublishDoesNotBlockOnFullSubscriber(t *testing.T) {
	hub := events.NewHub()
	lines, _ := hub.Subscribe()
	for range subscriberQueue + 1 {
		hub.Publish("cue tick")
	}
	if got := len(drain(lines)); got != subscriberQueue {
		t.Errorf("buffered = %d, want %d", got, subscriberQueue)
	}
}

func TestUnsubscribedChannelGetsNothing(t *testing.T) {
	hub := events.NewHub()
	lines, unsubscribe := hub.Subscribe()
	unsubscribe()
	hub.Publish("cue tick")
	if line, open := <-lines; open {
		t.Errorf("received %q after unsubscribe", line)
	}
}

func TestCueLine(t *testing.T) {
	if got := events.CueLine(synth.Spawn); got != "cue spawn" {
		t.Errorf("CueLine = %q, want %q", got, "cue spawn")
	}
}

func TestSettingsLine(t *testing.T) {
	if got := events.SettingsLine(control.Settings{VolumeDB: -6, Muted: true}); got != "settings volume -6 dB, muted" {
		t.Errorf("SettingsLine = %q, want %q", got, "settings volume -6 dB, muted")
	}
}

func TestDispatchCallsCueForCueLine(t *testing.T) {
	var got synth.Cue
	_ = events.Dispatch(events.CueLine(synth.Ask), func(c synth.Cue) { got = c }, ignoreSettings)
	if got != synth.Ask {
		t.Errorf("cue = %q, want %q", got, synth.Ask)
	}
}

func TestDispatchCallsSettingsForSettingsLine(t *testing.T) {
	want := control.Settings{VolumeDB: -12.5}
	var got control.Settings
	_ = events.Dispatch(events.SettingsLine(want), ignoreCue, func(s control.Settings) { got = s })
	if got != want {
		t.Errorf("settings = %+v, want %+v", got, want)
	}
}

func TestDispatchRejectsBadLines(t *testing.T) {
	for _, line := range []string{"", "volume -6 dB, muted", "settings loud"} {
		t.Run(line, func(t *testing.T) {
			if err := events.Dispatch(line, ignoreCue, ignoreSettings); !errors.Is(err, events.ErrLine) {
				t.Errorf("Dispatch(%q) error = %v, want %v", line, err, events.ErrLine)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail** — `make test`; Expected: FAIL, `meknik/events` has no non-test Go files.

- [ ] **Step 3: Implement** — `events/events.go`

```go
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
```

- [ ] **Step 4: Run tests** — `make test`, all `ok` including `meknik/events`.
- [ ] **Step 5: Lint** — `make lint`, `0 issues.`
- [ ] **Step 6: Commit**

```bash
fossil add events/events.go events/events_test.go
fossil changes
fossil commit -m "Add the events package that fans out and reads daemon event lines" events/events.go events/events_test.go
```

---

### Task 2: Events transport and socket name

**Files:**
- Create: `transport/events.go`, `transport/events_test.go`
- Modify: `transport/control.go`, `transport/control_test.go`, `statedir/statedir.go`, `statedir/statedir_test.go`, `cmd/meknik/main.go` (rename call site only)

**Interfaces:**
- Consumes: `events.Hub` (tests only).
- Produces:
  - `const statedir.EventsSocket = "meknik.events"`
  - `func transport.ListenStream(path string) (net.Listener, error)` (renamed from `ListenControl`; the `controlNetwork` const becomes `streamNetwork`)
  - `func transport.ServeEvents(l net.Listener, subscribe func() (<-chan string, func()), snapshot func() string) error`
  - `func transport.Watch(path string, line func(string)) error`

- [ ] **Step 1: Write the failing tests**

Append to `statedir/statedir_test.go`:

```go
func TestEventsSocketLivesInSharedRunDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/state")
	if got, _ := statedir.RunPath(statedir.EventsSocket); got != "/state/meknik/run/meknik.events" {
		t.Errorf("RunPath = %q, want /state/meknik/run/meknik.events", got)
	}
}
```

In `transport/control_test.go`, replace both `transport.ListenControl` calls with `transport.ListenStream`.

`transport/events_test.go`:

```go
package transport_test

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"meknik/events"
	"meknik/transport"
)

const (
	eventsName  = "meknik.events"
	snapshot    = "settings volume 0 dB, unmuted"
	eventWait   = time.Second
	publishWait = 50 * time.Millisecond
)

func serveEvents(t *testing.T, hub *events.Hub) (string, net.Listener) {
	t.Helper()
	path := filepath.Join(t.TempDir(), eventsName)
	l, err := transport.ListenStream(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() { _ = transport.ServeEvents(l, hub.Subscribe, func() string { return snapshot }) }()
	return path, l
}

func watch(path string) <-chan string {
	lines := make(chan string, 8)
	go func() { _ = transport.Watch(path, func(line string) { lines <- line }) }()
	return lines
}

func next(t *testing.T, lines <-chan string) string {
	t.Helper()
	select {
	case line := <-lines:
		return line
	case <-time.After(eventWait):
		return ""
	}
}

func TestWatchFirstReceivesSnapshot(t *testing.T) {
	path, _ := serveEvents(t, events.NewHub())
	if got := next(t, watch(path)); got != snapshot {
		t.Errorf("first line = %q, want %q", got, snapshot)
	}
}

func TestWatchReceivesPublishedLine(t *testing.T) {
	hub := events.NewHub()
	path, _ := serveEvents(t, hub)
	lines := watch(path)
	next(t, lines)
	hub.Publish("cue spawn")
	if got := next(t, lines); got != "cue spawn" {
		t.Errorf("line = %q, want %q", got, "cue spawn")
	}
}

func TestWatchReturnsWhenServerCloses(t *testing.T) {
	path := filepath.Join(t.TempDir(), eventsName)
	l, _ := transport.ListenStream(path)
	go func() {
		conn, _ := l.Accept()
		_ = conn.Close()
	}()
	done := make(chan error, 1)
	go func() { done <- transport.Watch(path, func(string) {}) }()
	select {
	case <-done:
	case <-time.After(eventWait):
		t.Error("Watch did not return after the server closed")
	}
}
```

(`publishWait` is unused if `next` suffices; delete it if lint flags it.)

- [ ] **Step 2: Run tests to verify they fail** — `make test`; Expected: FAIL, undefined `transport.ListenStream`, `transport.ServeEvents`, `transport.Watch`, `statedir.EventsSocket`.

- [ ] **Step 3: Implement**

`statedir/statedir.go`, beside the other socket names:

```go
// EventsSocket is the name of the daemon's events socket in the run directory.
const EventsSocket = "meknik.events"
```

`transport/control.go`: rename `ListenControl` to `ListenStream` with doc comment `// ListenStream binds a daemon stream socket, replacing any stale socket file.`; rename the const `controlNetwork` to `streamNetwork` everywhere in the package. In `cmd/meknik/main.go`, `transport.ListenControl(` becomes `transport.ListenStream(`.

`transport/events.go`:

```go
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
```

- [ ] **Step 4: Run tests** — `make test`, all `ok`.
- [ ] **Step 5: Lint** — `make lint`, `0 issues.`
- [ ] **Step 6: Commit**

```bash
fossil add transport/events.go transport/events_test.go
fossil changes
fossil commit -m "Add the events stream transport and socket name" transport/events.go transport/events_test.go transport/control.go transport/control_test.go statedir/statedir.go statedir/statedir_test.go cmd/meknik/main.go
```

---

### Task 3: Daemon publishes events

**Files:**
- Modify: `cmd/meknik/main.go`, `cmd/meknik/main_test.go`

**Interfaces:**
- Consumes: `events.NewHub`, `Hub.Publish`, `Hub.Subscribe`, `events.CueLine`, `events.SettingsLine`, `transport.ListenStream`, `transport.ServeEvents`, `statedir.EventsSocket`.
- Produces: `enqueue(sounds chan<- mixer.Sound, rng *rand.Rand, publish func(string)) func(synth.Cue)`.

- [ ] **Step 1: Write the failing tests**

In `cmd/meknik/main_test.go`, change the existing `enqueue(make(chan mixer.Sound), newRNG())` call in `TestEnqueueDropsWhenQueueIsFull` to `enqueue(make(chan mixer.Sound), newRNG(), func(string) {})`, and append:

```go
func TestEnqueuePublishesAcceptedCue(t *testing.T) {
	var published []string
	enqueue(make(chan mixer.Sound, 1), newRNG(), func(line string) { published = append(published, line) })(synth.Done)
	if want := []string{"cue done"}; !slices.Equal(published, want) {
		t.Errorf("published = %q, want %q", published, want)
	}
}

func TestEnqueuePublishesNothingForDroppedCue(t *testing.T) {
	var published []string
	enqueue(make(chan mixer.Sound), newRNG(), func(line string) { published = append(published, line) })(synth.Done)
	if len(published) != 0 {
		t.Errorf("published = %q, want none", published)
	}
}
```

Add `"slices"` to the test imports if missing.

- [ ] **Step 2: Run tests to verify they fail** — `make test`; Expected: FAIL, too many arguments in call to `enqueue`.

- [ ] **Step 3: Implement** in `cmd/meknik/main.go`

`enqueue` takes `publish func(string)` and publishes after the cue is accepted:

```go
func enqueue(sounds chan<- mixer.Sound, rng *rand.Rand, publish func(string)) func(synth.Cue) {
	return func(c synth.Cue) {
		samples, ok := synth.Sound(c, rng)
		if !ok {
			slog.Warn("unknown cue", "cue", c)
			return
		}
		select {
		case sounds <- mixer.Sound{Samples: samples, Tick: c == synth.Tick}:
			publish(events.CueLine(c))
		default:
			slog.Warn("sound queue full, cue dropped", "cue", c)
		}
	}
}
```

In `runDaemon`:

- After `controlSocket` is resolved, resolve `eventsSocket, err := statedir.RunPath(statedir.EventsSocket)` with the same `if err != nil { return err }`.
- After `listener` is bound, bind `eventsListener, err := transport.ListenStream(eventsSocket)`; on error `return errors.Join(err, conn.Close(), os.Remove(socket), listener.Close())`.
- The `playback.Start` error return also closes `eventsListener`.
- Create `hub := events.NewHub()` before the controller. The controller's apply function becomes `func(s control.Settings) { gains <- s.Gain(); hub.Publish(events.SettingsLine(s)) }`.
- `transport.Serve(conn, enqueue(sounds, newRNG(), hub.Publish))`.
- Add a goroutine like the control one:

```go
	go func() {
		snapshot := func() string {
			s, _ := controller.Handle(control.Request{Command: control.Status})
			return events.SettingsLine(s)
		}
		if err := transport.ServeEvents(eventsListener, hub.Subscribe, snapshot); err != nil {
			slog.Error("events socket failed", "err", err)
		}
	}()
```

- The final `return errors.Join(...)` also includes `eventsListener.Close()`.
- Import `"meknik/events"`.

- [ ] **Step 4: Run tests** — `make test`, all `ok`.
- [ ] **Step 5: Lint** — `make lint`, `0 issues.`
- [ ] **Step 6: Check the stream by hand**

Run: `make install && pkill -x meknik; meknik status && (timeout 3 socat -u UNIX-CONNECT:$HOME/.local/state/meknik/run/meknik.events - &) ; sleep 0.5; meknik volume -3; meknik play done; meknik volume 0; sleep 3`
Expected output includes `settings volume 0 dB, unmuted`, then `settings volume -3 dB, unmuted`, then `settings volume 0 dB, unmuted`. (`meknik play` bypasses the daemon, so no cue line is expected from it; use `printf done | socat - UNIX-SENDTO:$HOME/.local/state/meknik/run/meknik.sock` instead to see `cue done`.) If `socat` is missing, use `nc -U`.

- [ ] **Step 7: Commit**

```bash
fossil changes
fossil commit -m "Publish cue and settings events from the daemon" cmd/meknik/main.go cmd/meknik/main_test.go
```

---

### Task 4: Lamps, margin, and watcher in `meknik-ui`

**Files:**
- Create: `cmd/meknik-ui/lamps.go`, `cmd/meknik-ui/lamps_test.go`
- Modify: `cmd/meknik-ui/main.go`

**Interfaces:**
- Consumes: `transport.Watch`, `events.Dispatch`, `statedir.EventsSocket`, `synth.Cues`, the existing `panel.show`.
- Produces (package main): `lampColours map[synth.Cue]string`, `lampCSS() string`, `type lamps struct`, `newLamps() *lamps`, `(*lamps).light(synth.Cue)`.

- [ ] **Step 1: Write the failing tests** — `cmd/meknik-ui/lamps_test.go`

```go
package main

import (
	"strings"
	"testing"

	"meknik/synth"
)

func TestEveryCueHasALampColour(t *testing.T) {
	for _, c := range synth.Cues {
		t.Run(string(c), func(t *testing.T) {
			if _, ok := lampColours[c]; !ok {
				t.Errorf("no lamp colour for %q", c)
			}
		})
	}
}

func TestLampCSSLightsEachCueInItsColour(t *testing.T) {
	for _, c := range synth.Cues {
		t.Run(string(c), func(t *testing.T) {
			if rule := ".lamp-" + string(c) + ".lit { background-color: " + lampColours[c] + "; }"; !strings.Contains(lampCSS(), rule) {
				t.Errorf("lampCSS lacks %q", rule)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail** — `make test`; Expected: FAIL, undefined `lampColours`, `lampCSS`.

- [ ] **Step 3: Write `cmd/meknik-ui/lamps.go`**

```go
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"meknik/synth"
)

const (
	lampSize      = 14
	lampSpacing   = 8
	lampHold      = 40 * time.Millisecond
	lampClass     = "lamp"
	litClass      = "lit"
	lampDark      = "#1b2130"
	lampBaseRule  = ".lamp { min-width: %dpx; min-height: %dpx; border-radius: %dpx; background-color: %s; transition: background-color 200ms ease-out; }\n.lamp.lit { transition: none; }\n"
	lampColourRule = ".lamp-%s.lit { background-color: %s; }\n"
)

var lampColours = map[synth.Cue]string{
	synth.Tick:       "#ffcc99",
	synth.Spawn:      "#ff9966",
	synth.Return:     "#cc99cc",
	synth.Failed:     "#cc6666",
	synth.Permission: "#ffcc66",
	synth.Ask:        "#9999ff",
	synth.Reply:      "#99ccff",
	synth.Milestone:  "#ff9900",
	synth.Noted:      "#cc99ff",
	synth.Done:       "#99cc99",
}

type lamps struct {
	row     *gtk.Box
	lamp    map[synth.Cue]*gtk.Box
	pending map[synth.Cue]glib.SourceHandle
}

func lampCSS() string {
	var css strings.Builder
	fmt.Fprintf(&css, lampBaseRule, lampSize, lampSize, lampSize/2, lampDark)
	for _, c := range synth.Cues {
		fmt.Fprintf(&css, lampColourRule, c, lampColours[c])
	}
	return css.String()
}

func newLamps() *lamps {
	provider := gtk.NewCSSProvider()
	provider.LoadFromString(lampCSS())
	gtk.StyleContextAddProviderForDisplay(gdk.DisplayGetDefault(), provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
	l := &lamps{row: gtk.NewBox(gtk.OrientationHorizontal, lampSpacing), lamp: map[synth.Cue]*gtk.Box{}, pending: map[synth.Cue]glib.SourceHandle{}}
	l.row.SetHAlign(gtk.AlignCenter)
	for _, c := range synth.Cues {
		lamp := gtk.NewBox(gtk.OrientationHorizontal, 0)
		lamp.AddCSSClass(lampClass)
		lamp.AddCSSClass(lampClass + "-" + string(c))
		lamp.SetTooltipText(string(c))
		l.lamp[c] = lamp
		l.row.Append(lamp)
	}
	return l
}

func (l *lamps) light(c synth.Cue) {
	lamp, ok := l.lamp[c]
	if !ok {
		return
	}
	if l.pending[c] != 0 {
		glib.SourceRemove(l.pending[c])
	}
	lamp.AddCSSClass(litClass)
	l.pending[c] = glib.TimeoutAdd(uint(lampHold.Milliseconds()), func() {
		l.pending[c] = 0
		lamp.RemoveCSSClass(litClass)
	})
}
```

(If `gtk.AlignCenter` or `SetHAlign` differ in gotk4 v0.3.1, grep the module: `podman run --rm -v meknik-gomod:/root/go/pkg/mod localhost/meknik-build sh -c 'grep -rhn "AlignCenter\|func (widget \*Widget) SetHAlign" /root/go/pkg/mod/github.com/diamondburned/gotk4/pkg@v0.3.1/gtk/v4/ | head'`.)

- [ ] **Step 4: Run tests** — `make test`, all `ok`.

- [ ] **Step 5: Wire margin, lamps, and watcher into `cmd/meknik-ui/main.go`**

Constants: add `windowMargin = 12` and `reconnectInterval = time.Second`.

Panel field: `lamps *lamps`.

`main`: resolve `eventsPath, err := statedir.RunPath(statedir.EventsSocket)` after the logger is set (on error, print to stderr and exit 1 like the log error), and in the activate handler start the watcher:

```go
	app.ConnectActivate(func() {
		p := newPanel(app, askDaemon, func() error { return selftest.RunDaemon(spawnSibling) })
		go p.watch(eventsPath)
		p.send(control.Request{Command: control.Status})
	})
```

`newPanel`: set `lamps: newLamps()` in the literal; after `box := gtk.NewBox(...)`:

```go
	box.SetMarginTop(windowMargin)
	box.SetMarginBottom(windowMargin)
	box.SetMarginStart(windowMargin)
	box.SetMarginEnd(windowMargin)
	box.Append(p.lamps.row)
```

so the lamp row is the first child, above the volume scale.

Add:

```go
func (p *panel) watch(path string) {
	for {
		err := transport.Watch(path, func(line string) { glib.IdleAdd(func() { p.event(line) }) })
		slog.Warn("events stream ended", "err", err)
		time.Sleep(reconnectInterval)
	}
}

func (p *panel) event(line string) {
	if err := events.Dispatch(line, p.lamps.light, func(s control.Settings) { p.show(s, nil) }); err != nil {
		slog.Warn("unreadable event", "line", line, "err", err)
	}
}
```

Imports: add `"meknik/events"`.

- [ ] **Step 6: Build, test, lint** — `make build` (both binaries), `make test` all `ok`, `make lint` `0 issues.`

- [ ] **Step 7: Commit**

```bash
fossil add cmd/meknik-ui/lamps.go cmd/meknik-ui/lamps_test.go
fossil changes
fossil commit -m "Add cue lamps, an inner margin, and live settings to meknik-ui" cmd/meknik-ui/lamps.go cmd/meknik-ui/lamps_test.go cmd/meknik-ui/main.go
```

- [ ] **Step 8: Check by hand (controller, with the user)** — `make install`, close every `meknik-ui`, restart the daemon (`pkill -x meknik`), open `meknik-ui`:
1. The window has an even margin; lamps sit above the scale.
2. `meknik selftest` sweeps the lamps in order, each fading in about 200 ms.
3. `meknik mute`, `meknik volume -12` in a terminal move the window's controls.
4. Normal Claude Code activity flickers the tick lamp without stalling the window.
