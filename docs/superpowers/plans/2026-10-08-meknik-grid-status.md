# meknik grid status Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Control replies, and so `meknik status`, report tempo, division, queued and dropped counts from the grid scheduler.

**Architecture:** The scheduler publishes an atomic `grid.State` snapshot; `ServeControl` appends a `describe()` string to each reply after `; `; `transport.Control` splits it back off; the CLI prints both parts joined with `, `.

**Tech Stack:** Go 1.26 in the podman build image, Fossil.

**Spec:** `docs/superpowers/specs/2026-10-08-meknik-grid-status-design.md`

## Global Constraints

- Status text: `120 bpm in 16ths, 0 queued, 0 dropped` (division 16 or 32; `Dropped` counts every drop since start, never reset).
- Wire reply: `<Settings.String()>; <grid>`; separator exactly `; `.
- CLI line: `<Settings.String()>, <grid>`, e.g. `volume 0 dB, unmuted, 120 bpm in 32nds, 3 queued, 41 dropped`.
- The audio loop never blocks on a status read (atomic snapshot, no lock).
- Event `settings` lines, `meknik-ui`, and `selftest` behaviour unchanged.
- Test: `make test`. Lint: `make lint`. Both pass before each commit.
- Fossil: one-line commits, no trailers, each path listed; never git; never `fossil revert`/`fossil rm`.
- Code style per `~/.claude/CLAUDE.md`: no comments beyond one-line doc comments; one assertion per test; tests < 5 s; named constants; no defensive checks; no fallbacks; DRY.

## Review Focus

1. **Snapshot freshness**: the state is stored after every `Offer` and `Due` (including the drop branch and the slot switch inside `Due`), so status never lags more than one block. Checked by the Task 1 reviewer.
2. **Data race**: `State()` is called from the control goroutine while the audio loop mutates the scheduler; only the atomic pointer is shared. Checked by the Task 1 reviewer.
3. **Daemon refusal path**: an error reply (`error: …`) carries no grid part and still maps to `ErrDaemon`. Checked by the Task 2 reviewer.

---

### Task 1: `grid.State`

**Files:** Modify `grid/grid.go`, `grid/grid_test.go`.

**Interfaces — Produces:** `grid.State{Division, Queued, Dropped int}`; `(grid.State).String() string`; `(*grid.Scheduler).State() grid.State`.

- [ ] **Step 1: Write the failing tests** (append to `grid/grid_test.go`):

```go
func TestNewSchedulerReportsEmptySixteenths(t *testing.T) {
	if got, want := grid.New().State(), (grid.State{Division: 16}); got != want {
		t.Errorf("State = %+v, want %+v", got, want)
	}
}

func TestStateCountsQueuedSounds(t *testing.T) {
	s := grid.New()
	offers(s, 3)
	if got := s.State().Queued; got != 3 {
		t.Errorf("Queued = %d, want 3", got)
	}
}

func TestStateCountsDroppedSounds(t *testing.T) {
	s := grid.New()
	offers(s, queueLimit+2)
	if got := s.State().Dropped; got != 2 {
		t.Errorf("Dropped = %d, want 2", got)
	}
}

func TestStateReportsThirtySecondsAfterThirdDrop(t *testing.T) {
	s := grid.New()
	offers(s, queueLimit+3)
	if got := s.State().Division; got != 32 {
		t.Errorf("Division = %d, want 32", got)
	}
}

func TestStateString(t *testing.T) {
	if got, want := (grid.State{Division: 32, Queued: 3, Dropped: 41}).String(), "120 bpm in 32nds, 3 queued, 41 dropped"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run** `make test`; Expected: FAIL (undefined `grid.State`, `State`).

- [ ] **Step 3: Implement** in `grid/grid.go`:

Constants: add `framesPerWhole = beatsPerBar * framesPerBeat` and `stateFormat = "%d bpm in %dths, %d queued, %d dropped"`. Imports: `"fmt"`, `"sync/atomic"`.

```go
// State is a snapshot of the grid for status reports.
type State struct {
	Division int
	Queued   int
	Dropped  int
}

// String reports the tempo, division, and queue counts.
func (st State) String() string {
	return fmt.Sprintf(stateFormat, beatsPerMinute, st.Division, st.Queued, st.Dropped)
}
```

`Scheduler` gains `dropped int` and `state atomic.Pointer[State]`.

```go
// New returns a scheduler on a 16th-note grid at frame 0.
func New() *Scheduler {
	s := &Scheduler{slot: sixteenth}
	s.publish()
	return s
}

// State returns the latest snapshot; it is safe to call from any goroutine.
func (s *Scheduler) State() State { return *s.state.Load() }

func (s *Scheduler) publish() {
	s.state.Store(&State{Division: framesPerWhole / s.slot, Queued: len(s.pending), Dropped: s.dropped})
}
```

`Offer`: first statement `defer s.publish()`; in the full-queue branch add `s.dropped++` beside `s.drops++`. `Due`: first statement `defer s.publish()`.

- [ ] **Step 4:** `make test` all ok; `make lint` `0 issues.`
- [ ] **Step 5: Commit**

```bash
fossil changes
fossil commit -m "Publish a grid state snapshot with division, queued, and dropped counts" grid/grid.go grid/grid_test.go
```

---

### Task 2: Grid state in control replies and the CLI

**Files:** Modify `transport/control.go`, `transport/control_test.go`, `cmd/meknik/main.go`, `cmd/meknik/main_test.go`, `cmd/meknik-ui/main.go`, `selftest/selftest.go`.

**Interfaces — Consumes:** `(*grid.Scheduler).State()`, `grid.State.String`. **Produces:** `transport.ServeControl(l net.Listener, handle func(control.Request) (control.Settings, error), describe func() string) error`; `transport.Control(path string, r control.Request, spawn func() error) (control.Settings, string, error)`; in `cmd/meknik`: `statusLine(s control.Settings, gridState string) string`.

- [ ] **Step 1: Write the failing tests**

`transport/control_test.go`:
- Add const `gridState = "120 bpm in 16ths, 0 queued, 0 dropped"`.
- `serveControl` helper calls `transport.ServeControl(l, handle, func() string { return gridState })`.
- `TestAskReturnsDaemonStatus` wants `"volume -6 dB, unmuted; " + gridState`.
- `TestServeControlReturnsNilWhenClosed` passes `nil, nil`.
- Every `transport.Control(...)` call takes three results: `got, _, _ :=` / `_, _, err :=`.
- Append:

```go
func TestControlReturnsGridState(t *testing.T) {
	path := controlPath(t)
	serveControl(t, path, func(r control.Request) (control.Settings, error) { return control.Settings{}.Apply(r), nil })
	if _, got, _ := transport.Control(path, control.Request{Command: control.Status}, func() error { return nil }); got != gridState {
		t.Errorf("grid = %q, want %q", got, gridState)
	}
}
```

`cmd/meknik/main_test.go` — append:

```go
func TestStatusLineJoinsSettingsAndGrid(t *testing.T) {
	got := statusLine(control.Settings{VolumeDB: -6, Muted: true}, "120 bpm in 32nds, 3 queued, 41 dropped")
	if want := "volume -6 dB, muted, 120 bpm in 32nds, 3 queued, 41 dropped"; got != want {
		t.Errorf("statusLine = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run** `make test`; Expected: FAIL (too many arguments to `ServeControl`; assignment mismatch on `Control`; undefined `statusLine`).

- [ ] **Step 3: Implement**

`transport/control.go`:
- Const `statusSeparator = "; "`.
- `ServeControl`, `answer`, `reply`, `respond` take `describe func() string` through to `respond`, whose success return becomes `s.String() + statusSeparator + describe()`.
- `Control`'s doc comment: `// Control sends a request, starting a daemon with spawn when none answers, and returns the resulting settings and grid state.`; it returns `(control.Settings, string, error)`:

```go
	if err := Deliver(ask, spawn, SpawnTimeout, RetryInterval); err != nil {
		return control.Settings{}, "", err
	}
	settingsPart, gridState, _ := strings.Cut(status, statusSeparator)
	s, err := control.ParseSettings(settingsPart)
	return s, gridState, err
```

`cmd/meknik/main.go`:
- Const `statusJoin = ", "`; add

```go
func statusLine(s control.Settings, gridState string) string { return s.String() + statusJoin + gridState }
```

- `runControl`: `settings, gridState, err := transport.Control(path, r, spawnDaemon)` … `fmt.Println(statusLine(settings, gridState))`.
- `runDaemon`: create `sched := grid.New()` before the goroutines; `transport.ServeControl(listener, controller.Handle, func() string { return sched.State().String() })`; `daemon.Play(..., sched, ...)` in place of `grid.New()`.

`cmd/meknik-ui/main.go` `askDaemon`: `s, _, err := transport.Control(path, r, spawnSibling)`; `return s, err`.

`selftest/selftest.go` `RunDaemon`'s status func: `s, _, err := transport.Control(...)`; `return s, err`.

- [ ] **Step 4:** `make test` all ok; `make lint` `0 issues.`
- [ ] **Step 5: Commit**

```bash
fossil changes
fossil commit -m "Report grid state in control replies and meknik status" transport/control.go transport/control_test.go cmd/meknik/main.go cmd/meknik/main_test.go cmd/meknik-ui/main.go selftest/selftest.go
```

- [ ] **Step 6: Check by hand (controller)** — `make install && pkill -x meknik; meknik status` prints `volume …, unmuted, 120 bpm in 16ths, 0 queued, 0 dropped`; after a 10-tick burst, status shows `in 32nds` and `6 dropped`.
