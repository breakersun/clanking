# meknik tempo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `meknik tempo <bpm>` (60–240) sets a saved tempo that drives the grid; status shows it.

**Architecture:** Tempo is a `control.Settings` field (saved, defaulting to 120) and part of `Settings.String()`. The grid scheduler becomes tempo-aware (`New(bpm)`, `SetTempo`). `daemon.Play` receives whole settings values instead of gains and applies gain and tempo. The grid status drops its tempo; the CLI joins settings and grid with a space.

**Tech Stack:** Go 1.26 in the podman build image, Fossil.

**Spec:** `docs/superpowers/specs/2026-10-08-meknik-tempo-design.md`

## Global Constraints

- `control.Tempo = "tempo"`; `MinTempoBPM = 60`, `MaxTempoBPM = 240`, `DefaultTempoBPM = 120`; whole numbers only.
- `Settings.String()` = `volume %g dB` + `, ` + `muted|unmuted` + `, ` + `%d bpm`, e.g. `volume 0 dB, unmuted, 120 bpm`; `ParseSettings` requires exactly these three parts.
- `Settings.TempoBPM` is saved as `tempo_bpm`; `Load` defaults it to 120.
- Grid status: `in 16ths, 0 queued, 0 dropped`. CLI line: `<settings> <grid>` (space), e.g. `volume 0 dB, unmuted, 120 bpm in 16ths, 0 queued, 0 dropped`.
- Whole note = `4 * synth.SampleRate * 60 / bpm` frames; slot = whole / division (16 or 32); quiet stretch = one whole note.
- Test: `make test`. Lint: `make lint`. Both pass before each commit.
- Fossil: one-line commits, no trailers, each path listed; never git; never `fossil revert`/`fossil rm`.
- Code style per `~/.claude/CLAUDE.md`: no comments beyond one-line doc comments; one assertion per test; tests < 5 s; named constants; no defensive checks; no fallbacks; DRY.

## Review Focus

1. **Zero tempo**: `SetTempo(0)` divides by zero. Every path that reaches it must carry a valid tempo: `Load` default, `Parse` range check, `Apply`. Test fixtures that send `control.Settings{}` into `Play` would panic. Checked by the Task 3 reviewer.
2. **Tempo change mid-queue**: changing the slot length while sounds wait must not start two sounds on one line. Checked by the Task 2 reviewer.
3. **Old window, new daemon**: an open pre-change `meknik-ui` cannot read the new settings lines; accepted (restart the window). No reviewer action.

---

### Task 1: Tempo setting in `control`

**Files:** Modify `control/control.go`, `control/control_test.go`, `events/events_test.go`, `transport/events_test.go`, `transport/control_test.go`.

**Interfaces — Produces:** `control.Tempo`, `control.MinTempoBPM`, `control.MaxTempoBPM`, `control.DefaultTempoBPM`; `Request.TempoBPM int`; `Settings.TempoBPM int`.

- [ ] **Step 1: Write the failing tests**

In `control/control_test.go`:
- `TestParseAcceptsRequests` gains `"tempo 90": {Command: control.Tempo, TempoBPM: 90}`.
- `TestParseRejectsInvalidRequests` gains `"tempo 59"`, `"tempo 241"`, `"tempo fast"`, `"tempo 90.5"`.
- `TestSettingsStringReportsStatus` uses `control.Settings{VolumeDB: -6, Muted: true, TempoBPM: 90}` and wants `"volume -6 dB, muted, 90 bpm"`.
- `TestParseSettingsReadsStatusLine` / `…ReadsUnmuted` round-trip values gain `TempoBPM: 90` / `TempoBPM: 240`.
- `TestParseSettingsRejectsMalformedLines` list becomes `{"", "volume -6 dB, muted", "volume loud dB, muted, 90 bpm", "volume -6 dB, quiet, 90 bpm", "volume -6 dB, muted, fast bpm", "volume -6 dB, muted, 90 bpm, extra"}`.
- Append:

```go
func TestApplyTempo(t *testing.T) {
	got := control.Settings{TempoBPM: control.DefaultTempoBPM}.Apply(control.Request{Command: control.Tempo, TempoBPM: 90})
	if want := (control.Settings{TempoBPM: 90}); got != want {
		t.Errorf("Apply = %+v, want %+v", got, want)
	}
}

func TestLoadWithoutFileGivesDefaultTempo(t *testing.T) {
	if got, _ := control.Load(filepath.Join(t.TempDir(), "mixer.json")); got.TempoBPM != control.DefaultTempoBPM {
		t.Errorf("TempoBPM = %d, want %d", got.TempoBPM, control.DefaultTempoBPM)
	}
}

func TestLoadWithoutTempoFieldGivesDefaultTempo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixer.json")
	_ = os.WriteFile(path, []byte(`{"volume_db":-3,"muted":false}`), 0o644)
	if got, _ := control.Load(path); got.TempoBPM != control.DefaultTempoBPM {
		t.Errorf("TempoBPM = %d, want %d", got.TempoBPM, control.DefaultTempoBPM)
	}
}
```

(`TestLoadWithoutFileGivesDefaults` now wants `control.Settings{TempoBPM: control.DefaultTempoBPM}`.)

Other packages' tests that spell a settings line add the tempo:
- `events/events_test.go` `TestSettingsLine`: `Settings{VolumeDB: -6, Muted: true, TempoBPM: 120}` → `"settings volume -6 dB, muted, 120 bpm"`; `TestDispatchCallsSettingsForSettingsLine` `want` gains `TempoBPM: 120`.
- `transport/events_test.go` `snapshot` = `"settings volume 0 dB, unmuted, 120 bpm"`.
- `transport/control_test.go` `TestAskReturnsDaemonStatus` wants `"volume -6 dB, unmuted, 0 bpm; " + gridState` (its handler applies to `Settings{}`).

- [ ] **Step 2: Run** `make test`; Expected: FAIL (undefined `control.Tempo`, etc.).

- [ ] **Step 3: Implement** in `control/control.go`:

```go
const (
	Mute   = "mute"
	Unmute = "unmute"
	Volume = "volume"
	Status = "status"
	Toggle = "toggle"
	Tempo  = "tempo"
)

// The tempo range in beats per minute, and the tempo before any is set.
const (
	MinTempoBPM     = 60
	MaxTempoBPM     = 240
	DefaultTempoBPM = 120
)
```

Private constants: replace `statusFormat` with `volumeFormat = "volume %g dB"`, `tempoFormat = "%d bpm"`, `partSeparator = ", "`, `statusParts = 3`.

`Settings` gains `TempoBPM int \`json:"tempo_bpm"\``; `Request` gains `TempoBPM int`.

`Parse` gains a case:

```go
	case len(fields) == 2 && fields[0] == Tempo:
		bpm, err := strconv.Atoi(fields[1])
		if err == nil && bpm >= MinTempoBPM && bpm <= MaxTempoBPM {
			return Request{Command: Tempo, TempoBPM: bpm}, nil
		}
```

`Request.String`: `case Tempo: return fmt.Sprintf("%s %d", Tempo, r.TempoBPM)` (turn the `if` into a `switch`).

`Apply`: `case Tempo: s.TempoBPM = r.TempoBPM`.

```go
// String reports the settings as one status line.
func (s Settings) String() string {
	state := unmutedState
	if s.Muted {
		state = mutedState
	}
	return strings.Join([]string{fmt.Sprintf(volumeFormat, s.VolumeDB), state, fmt.Sprintf(tempoFormat, s.TempoBPM)}, partSeparator)
}

// ParseSettings reads a status line written by Settings.String.
func ParseSettings(line string) (Settings, error) {
	var s Settings
	parts := strings.Split(line, partSeparator)
	if len(parts) != statusParts || !slices.Contains([]string{mutedState, unmutedState}, parts[1]) {
		return Settings{}, fmt.Errorf("%w: %q", ErrStatus, line)
	}
	_, volumeErr := fmt.Sscanf(parts[0], volumeFormat, &s.VolumeDB)
	_, tempoErr := fmt.Sscanf(parts[2], tempoFormat, &s.TempoBPM)
	if volumeErr != nil || tempoErr != nil {
		return Settings{}, fmt.Errorf("%w: %q", ErrStatus, line)
	}
	s.Muted = parts[1] == mutedState
	return s, nil
}
```

`Load`:

```go
// Load reads saved settings over the defaults.
func Load(path string) (Settings, error) {
	s := Settings{TempoBPM: DefaultTempoBPM}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(data, &s)
}
```

- [ ] **Step 4:** `make test` all ok; `make lint` `0 issues.`
- [ ] **Step 5: Commit**

```bash
fossil changes
fossil commit -m "Add a saved tempo setting to control" control/control.go control/control_test.go events/events_test.go transport/events_test.go transport/control_test.go
```

---

### Task 2: Tempo-aware grid

**Files:** Modify `grid/grid.go`, `grid/grid_test.go`, `daemon/daemon_test.go`, `cmd/meknik/main.go` (the `grid.New()` call only).

**Interfaces — Consumes:** `control.DefaultTempoBPM` (tests). **Produces:** `grid.New(bpm int) *Scheduler`; `(*Scheduler).SetTempo(bpm int)`; `State.String()` = `in 16ths, 0 queued, 0 dropped`.

- [ ] **Step 1: Write the failing tests** in `grid/grid_test.go`:
- Add const `defaultTempo = 120` and `slowSixteenth = 8000`; every `grid.New()` becomes `grid.New(defaultTempo)`.
- `TestStateString` wants `"in 32nds, 3 queued, 41 dropped"`.
- Append:

```go
func TestSlowerTempoSpacesSixteenthsWider(t *testing.T) {
	s := grid.New(90)
	offers(s, 2)
	if got := starts(s, 2*slowSixteenth); len(got) != 2 || got[1] != slowSixteenth {
		t.Errorf("starts = %v, want [0 %d]", got, slowSixteenth)
	}
}

func TestSetTempoChangesSpacing(t *testing.T) {
	s := grid.New(defaultTempo)
	s.SetTempo(90)
	offers(s, 2)
	if got := starts(s, 2*slowSixteenth); len(got) != 2 || got[1] != slowSixteenth {
		t.Errorf("starts = %v, want [0 %d]", got, slowSixteenth)
	}
}

func TestDueRefreshesQueuedCount(t *testing.T) {
	s := grid.New(defaultTempo)
	offers(s, 2)
	s.Due(block)
	if got := s.State().Queued; got != 1 {
		t.Errorf("Queued = %d, want 1", got)
	}
}
```

In `daemon/daemon_test.go`, every `grid.New()` becomes `grid.New(control.DefaultTempoBPM)` (import `"meknik/control"`). In `cmd/meknik/main.go`, `grid.New()` becomes `grid.New(settings.TempoBPM)`.

- [ ] **Step 2: Run** `make test`; Expected: FAIL (too many arguments to `grid.New`).

- [ ] **Step 3: Implement** `grid/grid.go`:

```go
const (
	secondsPerMinute  = 60
	beatsPerBar       = 4
	sixteenths        = 16
	thirtySeconds     = 32
	stateFormat       = "in %s, %d queued, %d dropped"
	queueLimit        = 4
	dropsBeforeFaster = 2
)

var divisionNames = map[int]string{sixteenths: "16ths", thirtySeconds: "32nds"}
```

`State.String()`: `fmt.Sprintf(stateFormat, divisionNames[st.Division], st.Queued, st.Dropped)`; doc comment `// String reports the division and queue counts.`

`Scheduler`: replace `slot int` with `division int` and `whole int` (frames per whole note).

```go
// New returns a scheduler on a 16th-note grid at frame 0, at bpm beats per minute.
func New(bpm int) *Scheduler {
	s := &Scheduler{division: sixteenths}
	s.SetTempo(bpm)
	return s
}

// SetTempo sets the grid to bpm beats per minute.
func (s *Scheduler) SetTempo(bpm int) {
	s.whole = beatsPerBar * synth.SampleRate * secondsPerMinute / bpm
	s.publish()
}

func (s *Scheduler) slot() int { return s.whole / s.division }
```

- `publish`: `Division: s.division`.
- `Offer`: `s.slot = thirtySecond` → `s.division = thirtySeconds`.
- `Due`: the quiet check becomes `if s.division == thirtySeconds && s.clock-s.lastArrival >= s.whole { s.division = sixteenths }`; `s.free = line + s.slot()`.
- `nextLine`: `slot := s.slot()`; `return (from + slot - 1) / slot * slot`.

- [ ] **Step 4:** `make test` all ok; `make lint` `0 issues.`
- [ ] **Step 5: Commit**

```bash
fossil changes
fossil commit -m "Make the grid scheduler tempo-aware" grid/grid.go grid/grid_test.go daemon/daemon_test.go cmd/meknik/main.go
```

---

### Task 3: Daemon applies tempo; `meknik tempo`

**Files:** Modify `daemon/daemon.go`, `daemon/daemon_test.go`, `cmd/meknik/main.go`, `cmd/meknik/main_test.go`.

**Interfaces — Consumes:** `(*grid.Scheduler).SetTempo`, `control.Settings.Gain`, `control.Tempo`. **Produces:** `daemon.Play(m *mixer.Mixer, s *grid.Scheduler, sounds <-chan mixer.Sound, settings <-chan control.Settings, out io.Writer, idle time.Duration, now func() time.Time, started func(mixer.Sound)) error`.

- [ ] **Step 1: Write the failing tests**

`daemon/daemon_test.go`: every `make(chan float64)` becomes `make(chan control.Settings)`. `TestPlayAppliesQueuedGain` sends `control.Settings{Muted: true, TempoBPM: control.DefaultTempoBPM}` on a `chan control.Settings` (gain 0) and keeps its assertion. Append:

```go
const slowSixteenth = 8000

func TestPlayAppliesQueuedTempo(t *testing.T) {
	s := grid.New(control.DefaultTempoBPM)
	settings := make(chan control.Settings, 1)
	settings <- control.Settings{TempoBPM: 90}
	_ = daemon.Play(mixer.New(unity, wideCeiling), s, make(chan mixer.Sound), settings, &bytes.Buffer{}, idle, steppingClock(), func(mixer.Sound) {})
	s.Offer(mixer.Sound{Cue: synth.Done})
	s.Offer(mixer.Sound{Cue: synth.Done})
	var at []int
	for clock := 0; clock < 2*slowSixteenth; clock += daemon.BlockFrames {
		for _, st := range s.Due(daemon.BlockFrames) {
			at = append(at, clock+st.Offset)
		}
	}
	if len(at) != 2 || at[1]-at[0] != slowSixteenth {
		t.Errorf("starts = %v, want %d apart", at, slowSixteenth)
	}
}
```

`cmd/meknik/main_test.go`:
- `TestStatusLineJoinsSettingsAndGrid`: `statusLine(control.Settings{VolumeDB: -6, Muted: true, TempoBPM: 120}, "in 32nds, 3 queued, 41 dropped")` wants `"volume -6 dB, muted, 120 bpm in 32nds, 3 queued, 41 dropped"`.
- Append:

```go
func TestRunRejectsOutOfRangeTempoBeforeContactingDaemon(t *testing.T) {
	if err := run([]string{"tempo", "300"}); !errors.Is(err, control.ErrInvalid) {
		t.Errorf("run error = %v, want %v", err, control.ErrInvalid)
	}
}

func TestUsageListsTempo(t *testing.T) {
	if !strings.Contains(errUsage.Error(), "| tempo <bpm>") {
		t.Errorf("usage = %q, want it to list tempo", errUsage)
	}
}
```

- [ ] **Step 2: Run** `make test`; Expected: FAIL.

- [ ] **Step 3: Implement**

`daemon/daemon.go`: `Play`'s `gains <-chan float64` becomes `settings <-chan control.Settings`; `drain` likewise, with

```go
		case st := <-settings:
			m.SetGain(st.Gain())
			s.SetTempo(st.TempoBPM)
```

and its doc comment says "applying queued settings".

`cmd/meknik/main.go`:
- `gains := make(chan float64, soundQueue)` → `changes := make(chan control.Settings, soundQueue)`; the controller's apply becomes `func(s control.Settings) { changes <- s; hub.Publish(events.SettingsLine(s)) }`; `daemon.Play(..., sched, sounds, changes, ...)`.
- `run`: the volume case becomes `case len(args) == 2 && slices.Contains([]string{control.Volume, control.Tempo}, args[0]):`.
- `errUsage` gains ` | tempo <bpm>` after `volume <dB>`.
- `statusJoin` = `" "`.

- [ ] **Step 4:** `make test` all ok; `make lint` `0 issues.`
- [ ] **Step 5: Commit**

```bash
fossil changes
fossil commit -m "Apply tempo in the daemon and add meknik tempo" daemon/daemon.go daemon/daemon_test.go cmd/meknik/main.go cmd/meknik/main_test.go
```

- [ ] **Step 6: Check by hand (controller)** — `make install && pkill -x meknik`; `meknik status` → `… 120 bpm in 16ths …`; `meknik tempo 90` → `… 90 bpm in 16ths …`; `meknik tempo 300` → usage error; restart daemon → still 90; fire `return`+`done` → events ≈167 ms apart; `meknik tempo 120` to restore.
