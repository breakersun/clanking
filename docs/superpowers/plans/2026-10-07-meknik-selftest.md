# meknik self-test Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `meknik selftest` and a `Self-test` button in `meknik-ui` that play every cue once, in order, through the daemon, and refuse while muted.

**Architecture:** A new GTK-free `selftest` package sequences the cues with injected `status`, `send`, and `sleep` functions, and a `RunDaemon` wrapper wires them to the real sockets. The CLI and the window each call `RunDaemon` with their own spawn function. The daemon and the control protocol do not change.

**Tech Stack:** Go 1.26 in the podman build image, gotk4 v0.3.1, Fossil.

**Spec:** `docs/superpowers/specs/2026-10-07-meknik-selftest-design.md`

## Global Constraints

- Mute is always honored: while muted, the self-test returns `selftest.ErrMuted` ("muted; unmute to run the self-test") and sends no cue.
- Cue order is `synth.Cues`: tick, spawn, return, failed, permission, ask, reply, milestone, noted, done.
- Wait after each cue = the cue's sample count / `synth.SampleRate` + `cuePause` (400 ms).
- Cues go over the cue socket with `transport.Send`, the hook path.
- No title bar, no About window, no daemon or protocol change.
- Test: `make test`. Lint: `make lint`. Both must pass before each commit.
- Fossil commits, one line, no trailers, each path listed; `fossil add` new files first.
- Code style per `~/.claude/CLAUDE.md`: no comments beyond one-line doc comments; one assertion per test; tests < 5 s; named constants; no defensive checks; no fallbacks.

## Review Focus

1. **Daemon not running when the self-test starts**: `status` goes through `transport.Control`, which starts it; the cues then reach the new daemon. Checked by hand in Task 3, Step 6.
2. **Mute toggled during a test**: the remaining cues are silent, through the mixer; the test does not stop early. Checked by hand in Task 4, Step 6.
3. **Cue send fails mid-test** (daemon killed): `Run` returns the send error at once and sends nothing more. Pinned by `TestRunStopsOnSendError` in Task 2.
4. **Self-test button pressed twice**: the button is insensitive while a test runs, so a second sequence cannot overlap. Checked by hand in Task 4, Step 6.
5. **Mute or volume clicked during a UI self-test**: they go through the request worker at once, not after the test. Checked by hand in Task 4, Step 6.

---

## File Structure

| Path | Responsibility |
|---|---|
| `synth/cue.go`, `synth/cue_test.go` | Gains `Cues`. Modified. |
| `statedir/statedir.go`, `statedir/statedir_test.go` | Gains `CueSocket`. Modified. |
| `selftest/selftest.go`, `selftest/selftest_test.go` | `Run`, `RunDaemon`, `ErrMuted`. New. |
| `cmd/meknik/main.go`, `cmd/meknik/main_test.go` | `selftest` command; uses `statedir.CueSocket`. Modified. |
| `cmd/meknik-ui/main.go` | `Self-test` button. Modified. |

---

### Task 1: `synth.Cues`

**Files:**
- Modify: `synth/cue.go`, `synth/cue_test.go`

**Interfaces:**
- Produces: `var synth.Cues = []synth.Cue{Tick, Spawn, Return, Failed, Permission, Ask, Reply, Milestone, Noted, Done}`

- [ ] **Step 1: Write the failing tests** (append to `synth/cue_test.go`; add `"slices"` to its imports)

```go
func TestCuesListsEveryCueEndingOnDone(t *testing.T) {
	want := []synth.Cue{synth.Tick, synth.Spawn, synth.Return, synth.Failed, synth.Permission, synth.Ask, synth.Reply, synth.Milestone, synth.Noted, synth.Done}
	if !slices.Equal(synth.Cues, want) {
		t.Errorf("Cues = %v, want %v", synth.Cues, want)
	}
}

func TestEveryListedCueHasASound(t *testing.T) {
	rng := newRNG()
	for _, c := range synth.Cues {
		t.Run(string(c), func(t *testing.T) {
			if _, ok := synth.Sound(c, rng); !ok {
				t.Errorf("Sound(%q) unknown", c)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `make test`
Expected: FAIL — undefined `synth.Cues`.

- [ ] **Step 3: Implement** (in `synth/cue.go`, after the `Cue` constants)

```go
// Cues lists every cue in self-test order, ending on Done.
var Cues = []Cue{Tick, Spawn, Return, Failed, Permission, Ask, Reply, Milestone, Noted, Done}
```

- [ ] **Step 4: Run tests to verify they pass** — `make test`, all `ok`.
- [ ] **Step 5: Lint** — `make lint`, `0 issues.`
- [ ] **Step 6: Commit**

```bash
fossil changes
fossil commit -m "List every cue in self-test order" synth/cue.go synth/cue_test.go
```

---

### Task 2: `selftest` package and `statedir.CueSocket`

**Files:**
- Create: `selftest/selftest.go`, `selftest/selftest_test.go`
- Modify: `statedir/statedir.go`, `statedir/statedir_test.go`, `cmd/meknik/main.go`

**Interfaces:**
- Consumes: `synth.Cues`, `synth.Sound`, `synth.SampleRate`, `transport.Control`, `transport.Send`, `statedir.RunPath`, `statedir.ControlSocket`.
- Produces:
  - `const statedir.CueSocket = "meknik.sock"`
  - `var selftest.ErrMuted = errors.New("muted; unmute to run the self-test")`
  - `func selftest.Run(status func() (control.Settings, error), send func(synth.Cue) error, sleep func(time.Duration)) error`
  - `func selftest.RunDaemon(spawn func() error) error` — `Run` wired to the control socket, the cue socket, and `time.Sleep`.

- [ ] **Step 1: Write the failing tests**

Append to `statedir/statedir_test.go`:

```go
func TestCueSocketLivesInSharedRunDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/state")
	if got, _ := statedir.RunPath(statedir.CueSocket); got != "/state/meknik/run/meknik.sock" {
		t.Errorf("RunPath = %q, want /state/meknik/run/meknik.sock", got)
	}
}
```

`selftest/selftest_test.go`:

```go
package selftest_test

import (
	"errors"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"meknik/control"
	"meknik/selftest"
	"meknik/synth"
)

const cuePause = 400 * time.Millisecond

var (
	errStatus = errors.New("status failed")
	errSend   = errors.New("send failed")
)

func unmuted() (control.Settings, error) { return control.Settings{}, nil }
func muted() (control.Settings, error)   { return control.Settings{Muted: true}, nil }
func noSleep(time.Duration)              {}

func record(sent *[]synth.Cue) func(synth.Cue) error {
	return func(c synth.Cue) error { *sent = append(*sent, c); return nil }
}

func TestRunSendsCuesInOrder(t *testing.T) {
	var sent []synth.Cue
	_ = selftest.Run(unmuted, record(&sent), noSleep)
	if !slices.Equal(sent, synth.Cues) {
		t.Errorf("sent = %v, want %v", sent, synth.Cues)
	}
}

func TestRunWaitsForEachCueAndAPause(t *testing.T) {
	var waits []time.Duration
	_ = selftest.Run(unmuted, record(new([]synth.Cue)), func(d time.Duration) { waits = append(waits, d) })
	samples, _ := synth.Sound(synth.Cues[len(synth.Cues)-1], rand.New(rand.NewPCG(1, 2)))
	if want := time.Duration(len(samples))*time.Second/synth.SampleRate + cuePause; waits[len(waits)-1] != want {
		t.Errorf("last wait = %v, want %v", waits[len(waits)-1], want)
	}
}

func TestRunRefusesWhileMuted(t *testing.T) {
	if err := selftest.Run(muted, record(new([]synth.Cue)), noSleep); !errors.Is(err, selftest.ErrMuted) {
		t.Errorf("Run error = %v, want %v", err, selftest.ErrMuted)
	}
}

func TestRunSendsNothingWhileMuted(t *testing.T) {
	var sent []synth.Cue
	_ = selftest.Run(muted, record(&sent), noSleep)
	if len(sent) != 0 {
		t.Errorf("sent = %v, want none", sent)
	}
}

func TestRunReturnsStatusError(t *testing.T) {
	status := func() (control.Settings, error) { return control.Settings{}, errStatus }
	if err := selftest.Run(status, record(new([]synth.Cue)), noSleep); !errors.Is(err, errStatus) {
		t.Errorf("Run error = %v, want %v", err, errStatus)
	}
}

func TestRunStopsOnSendError(t *testing.T) {
	sends := 0
	_ = selftest.Run(unmuted, func(synth.Cue) error { sends++; return errSend }, noSleep)
	if sends != 1 {
		t.Errorf("sends = %d, want 1", sends)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `make test`
Expected: FAIL — `meknik/selftest` has no non-test Go files; undefined `statedir.CueSocket`.

- [ ] **Step 3: Implement**

In `statedir/statedir.go`, beside `ControlSocket`:

```go
// CueSocket is the name of the daemon's cue socket in the run directory.
const CueSocket = "meknik.sock"
```

In `cmd/meknik/main.go`, delete `socketName` and replace its two uses with `statedir.CueSocket`.

`selftest/selftest.go`:

```go
// Package selftest plays every cue once through the daemon, as a check that sound works.
package selftest

import (
	"errors"
	"math/rand/v2"
	"time"

	"meknik/control"
	"meknik/statedir"
	"meknik/synth"
	"meknik/transport"
)

const cuePause = 400 * time.Millisecond

// ErrMuted reports a self-test refused because meknik is muted.
var ErrMuted = errors.New("muted; unmute to run the self-test")

// Run plays every cue once through send, waiting for each to finish, and refuses while muted.
func Run(status func() (control.Settings, error), send func(synth.Cue) error, sleep func(time.Duration)) error {
	settings, err := status()
	if err != nil {
		return err
	}
	if settings.Muted {
		return ErrMuted
	}
	rng := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	for _, c := range synth.Cues {
		if err := send(c); err != nil {
			return err
		}
		samples, _ := synth.Sound(c, rng)
		sleep(time.Duration(len(samples))*time.Second/synth.SampleRate + cuePause)
	}
	return nil
}

// RunDaemon runs the self-test against the daemon, starting one with spawn when none answers.
func RunDaemon(spawn func() error) error {
	controlPath, err := statedir.RunPath(statedir.ControlSocket)
	if err != nil {
		return err
	}
	cuePath, err := statedir.RunPath(statedir.CueSocket)
	if err != nil {
		return err
	}
	status := func() (control.Settings, error) {
		return transport.Control(controlPath, control.Request{Command: control.Status}, spawn)
	}
	return Run(status, func(c synth.Cue) error { return transport.Send(cuePath, c) }, time.Sleep)
}
```

- [ ] **Step 4: Run tests to verify they pass** — `make test`, all `ok` including `meknik/selftest`.
- [ ] **Step 5: Lint** — `make lint`, `0 issues.`
- [ ] **Step 6: Commit**

```bash
fossil add selftest/selftest.go selftest/selftest_test.go
fossil changes
fossil commit -m "Add the selftest package that plays every cue through the daemon" selftest/selftest.go selftest/selftest_test.go statedir/statedir.go statedir/statedir_test.go cmd/meknik/main.go
```

---

### Task 3: `meknik selftest`

**Files:**
- Modify: `cmd/meknik/main.go`, `cmd/meknik/main_test.go`

**Interfaces:**
- Consumes: `selftest.RunDaemon(spawn func() error) error`.

- [ ] **Step 1: Write the failing test** (append to `cmd/meknik/main_test.go`)

```go
func TestUsageListsSelftest(t *testing.T) {
	if !strings.Contains(errUsage.Error(), "| selftest") {
		t.Errorf("usage = %q, want it to list selftest", errUsage)
	}
}
```

Add `"strings"` to the test imports.

- [ ] **Step 2: Run test to verify it fails** — `make test`; Expected: FAIL in `TestUsageListsSelftest`.

- [ ] **Step 3: Implement**

In `run`, add a case before the final `return errUsage`:

```go
	case slices.Equal(args, []string{"selftest"}):
		return selftest.RunDaemon(spawnDaemon)
```

Append ` | selftest` to the `errUsage` text, and add `"meknik/selftest"` to the imports.

- [ ] **Step 4: Run tests** — `make test`, all `ok`.
- [ ] **Step 5: Lint** — `make lint`, `0 issues.`
- [ ] **Step 6: Install and check by hand**

Run: `make install && pkill -x meknik; meknik unmute && time meknik selftest; echo exit=$?`
Expected: every cue audible once in order; `exit=0`; about 5–10 s.

Run: `meknik mute; meknik selftest; echo exit=$?; meknik unmute`
Expected: `meknik: muted; unmute to run the self-test`, `exit=1`.

- [ ] **Step 7: Commit**

```bash
fossil changes
fossil commit -m "Add meknik selftest" cmd/meknik/main.go cmd/meknik/main_test.go
```

---

### Task 4: `Self-test` button

**Files:**
- Modify: `cmd/meknik-ui/main.go`

**Interfaces:**
- Consumes: `selftest.RunDaemon`, `spawnSibling` (already in `cmd/meknik-ui`).

The GTK wiring is checked by hand (spec, Testing).

- [ ] **Step 1: Add the button**

Constants: `selfTestLabel = "Self-test"`. Panel field: `selfTest *gtk.Button`, and `test func() error`. `newPanel` gains a `test func() error` parameter; `main` passes `func() error { return selftest.RunDaemon(spawnSibling) }`.

In `newPanel`:

```go
	p.selfTest = gtk.NewButtonWithLabel(selfTestLabel)
	p.selfTest.ConnectClicked(p.runSelfTest)
	box.Append(p.selfTest)
```

appended after `box.Append(p.mute)`.

```go
func (p *panel) runSelfTest() {
	p.selfTest.SetSensitive(false)
	go func() {
		err := p.test()
		glib.IdleAdd(func() {
			p.selfTest.SetSensitive(true)
			p.showError(err)
		})
	}()
}

func (p *panel) showError(err error) {
	if err != nil {
		slog.Error("meknik-ui request failed", "err", err)
		p.errors.SetText(err.Error())
		return
	}
	p.errors.SetText("")
}
```

and `show` uses `showError`:

```go
func (p *panel) show(settings control.Settings, err error) {
	p.showError(err)
	if err != nil {
		return
	}
	p.syncing = true
	p.volume.SetValue(settings.VolumeDB)
	p.mute.SetActive(settings.Muted)
	p.syncing = false
}
```

- [ ] **Step 2: Build** — `make build`; Expected: both binaries built.
- [ ] **Step 3: Run tests** — `make test`, all `ok`.
- [ ] **Step 4: Lint** — `make lint`, `0 issues.`
- [ ] **Step 5: Install** — `make install`. Close every running `meknik-ui` first: a second launch hands off to the running instance.
- [ ] **Step 6: Check by hand (with the user)**

1. Press Self-test. Expected: every cue once in order; the button is greyed until the last cue ends.
2. Press Self-test, and click Mute during the sequence. Expected: Mute takes effect at once; the remaining cues are silent; the button re-enables at the end.
3. With Mute pressed, press Self-test. Expected: error label shows `muted; unmute to run the self-test`.

- [ ] **Step 7: Commit**

```bash
fossil changes
fossil commit -m "Add a Self-test button to meknik-ui" cmd/meknik-ui/main.go
```
