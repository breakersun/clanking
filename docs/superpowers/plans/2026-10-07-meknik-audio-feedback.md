# meknik Audio Feedback Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `meknik`, a Go binary that turns Claude Code hook events into subtle, synthesized audio cues played through a self-starting daemon.

**Architecture:** `meknik hook` maps hook JSON to a cue name and sends it as a datagram to a Unix socket. If no daemon answers, it spawns `meknik daemon` and retries. The daemon, which is single-instance through `flock`, renders cues with pure synthesis code, mixes them through a voice pool and a tanh limiter, and streams float32 PCM to one long-lived `pw-cat`.

**Tech Stack:** Go 1.26, the standard library, `golang.org/x/sys/unix`, PipeWire `pw-cat`, golangci-lint 2.12.2, Fossil for version control.

**Spec:** `docs/superpowers/specs/2026-10-07-meknik-audio-feedback-design.md`

## Global Constraints

- Version control is **Fossil**, not git. Commit with `fossil add <new files>` and then `fossil commit -m "<msg>" <every touched file>`. Run `fossil changes` before each commit.
- Before every commit, `make lint` and `make test` must both pass with no warnings.
- Code style: no comments except one-line doc comments on exported identifiers and one package comment per package. Use named constants instead of literals. Inject dependencies as arguments. Don't check parameters defensively. Return errors to `main`, which logs them with `log/slog` and exits 1.
- Tests: exactly one assertion per test (table subtests have one assertion each), every test under 5 s, and no audio device needed.
- Audio format: 48 kHz, mono, float32 little-endian.
- Levels: `tick` peaks at −24 dBFS with ±3 dB random jitter, the other cues peak at −18 dBFS, and the limiter ceiling is −12 dBFS. `MEKNIK_GAIN` (dB, default 0) shifts the master gain.
- `MaxVoices` = 8, `BlockFrames` = 256, `IdleTimeout` = 10 min, `SpawnTimeout` = 1 s.
- Tonic is D5 = 587.33 Hz. Pitches are equal-tempered semitones above the tonic.
- Socket: `$XDG_RUNTIME_DIR/meknik.sock`. Lock: `$XDG_RUNTIME_DIR/meknik.lock`. Log: `$XDG_STATE_HOME/meknik/meknik.log` (default `~/.local/state/meknik/meknik.log`).
- The binary installs to `~/.local/bin/meknik`. Don't let `go install` use its default GOBIN, which is mise's Go directory here.

## Review Focus

1. **Stale socket after a daemon crash:** the socket file remains but nobody is listening. The next hook must still bring up a new daemon, and that daemon must bind over the stale file. Covered by `TestListenReplacesStaleSocket` (Task 5).
2. **A burst of hooks all spawning daemons at once:** exactly one daemon must survive and the others must exit quietly. Covered by `TestLockExcludesSecondHolder` (Task 6).
3. **pw-cat missing or dying:** the daemon must exit with a logged error rather than spin. Covered by `TestPlayReportsWriteFailure` (Task 6).
4. **Malformed or empty hook stdin:** the client must fail with a logged error, never a hang or a sound. Covered by `TestParseRejectsMalformedJSON` (Task 3).
5. **Many agents ticking at once:** cues must still be heard through a flood of ticks, and the total level must stay under the ceiling. Covered by the drop-policy and ceiling tests (Task 4).

---

### Task 1: Scaffold and tone synthesis

**Files:**
- Create: `go.mod`, `Makefile`, `mise.toml`, `.golangci.yml`
- Create: `internal/approx/approx.go`
- Create: `synth/tone.go`
- Test: `synth/tone_test.go`

**Interfaces:**
- Produces:
  - `approx.Equal(want, got, tolerance float64) bool` (relative tolerance)
  - `synth.SampleRate = 48000`, `synth.TonicHz = 587.33`
  - `type synth.Partial struct{ Harmonic, Amplitude float64 }`, `synth.Sine`, `synth.Buzzy []Partial`
  - `type synth.Note struct{ From, To float64; Onset, Length, Decay time.Duration; Timbre []Partial }`
  - `synth.Amplitude(db float64) float64`, `synth.Hz(semitones float64) float64`, `synth.Semitones(hz float64) float64`
  - `synth.Render(score []Note, peakDB float64) []float32`

- [ ] **Step 1: Scaffold the module and tooling**

```bash
cd /home/rc/meknik && go mod init meknik
```

`Makefile`:

```make
.PHONY: test lint install

test:
	go test ./...

lint:
	golangci-lint run ./...

install:
	GOBIN=$(HOME)/.local/bin go install ./cmd/meknik
```

`mise.toml`:

```toml
[tools]
go = "1.26.7"
golangci-lint = "2.12.2"
```

`.golangci.yml`:

```yaml
version: "2"
linters:
  enable:
    - revive
    - gocritic
    - errorlint
    - unparam
    - misspell
```

`internal/approx/approx.go`:

```go
// Package approx compares floating-point results in tests.
package approx

import "math"

// Equal reports whether got is within a relative tolerance of want.
func Equal(want, got, tolerance float64) bool {
	return math.Abs(got-want) <= tolerance*math.Abs(want)
}
```

- [ ] **Step 2: Write the failing tests**

`synth/tone_test.go`:

```go
package synth_test

import (
	"math"
	"testing"
	"time"

	"meknik/internal/approx"
	"meknik/synth"
)

const (
	testPeakDB     = -18.0
	pitchTolerance = 0.005
	levelTolerance = 1e-6
	sustain        = time.Hour
)

func ms(n int) int { return n * synth.SampleRate / 1000 }

func pitchHz(samples []float32) float64 {
	var first, last, count int
	for i := 1; i < len(samples); i++ {
		if (samples[i-1] < 0) != (samples[i] < 0) {
			if count == 0 {
				first = i
			}
			last = i
			count++
		}
	}
	return float64(count-1) / 2 / (float64(last-first) / synth.SampleRate)
}

func peak(samples []float32) float64 {
	loudest := 0.0
	for _, x := range samples {
		loudest = max(loudest, math.Abs(float64(x)))
	}
	return loudest
}

func steady(semitones float64, length time.Duration) []synth.Note {
	return []synth.Note{{From: semitones, To: semitones, Length: length, Decay: sustain, Timbre: synth.Sine}}
}

func TestAmplitudeOfMinusTwentyDecibels(t *testing.T) {
	if got := synth.Amplitude(-20); !approx.Equal(0.1, got, levelTolerance) {
		t.Errorf("Amplitude(-20) = %v, want 0.1", got)
	}
}

func TestSemitonesInvertsHz(t *testing.T) {
	if got := synth.Semitones(synth.Hz(7)); !approx.Equal(7, got, levelTolerance) {
		t.Errorf("Semitones(Hz(7)) = %v, want 7", got)
	}
}

func TestRenderStartsSilent(t *testing.T) {
	if got := synth.Render(steady(0, 50*time.Millisecond), testPeakDB)[0]; got != 0 {
		t.Errorf("first sample = %v, want 0", got)
	}
}

func TestRenderEndsSilent(t *testing.T) {
	samples := synth.Render(steady(0, 50*time.Millisecond), testPeakDB)
	if got := samples[len(samples)-1]; got != 0 {
		t.Errorf("last sample = %v, want 0", got)
	}
}

func TestRenderLengthCoversLatestNote(t *testing.T) {
	score := []synth.Note{
		{From: 0, To: 0, Length: 50 * time.Millisecond, Decay: sustain, Timbre: synth.Sine},
		{From: 0, To: 0, Onset: 30 * time.Millisecond, Length: 50 * time.Millisecond, Decay: sustain, Timbre: synth.Sine},
	}
	if got := len(synth.Render(score, testPeakDB)); got != ms(80) {
		t.Errorf("len = %d, want %d", got, ms(80))
	}
}

func TestRenderPeaksAtRequestedLevel(t *testing.T) {
	got := peak(synth.Render(steady(0, 50*time.Millisecond), testPeakDB))
	if !approx.Equal(synth.Amplitude(testPeakDB), got, levelTolerance) {
		t.Errorf("peak = %v, want %v", got, synth.Amplitude(testPeakDB))
	}
}

func TestRenderSteadyNoteSoundsAtTonic(t *testing.T) {
	if got := pitchHz(synth.Render(steady(0, 200*time.Millisecond), testPeakDB)); !approx.Equal(synth.TonicHz, got, pitchTolerance) {
		t.Errorf("pitch = %v, want %v", got, synth.TonicHz)
	}
}

func TestRenderGlideApproachesTarget(t *testing.T) {
	glide := []synth.Note{{From: 0, To: 7, Length: 250 * time.Millisecond, Decay: sustain, Timbre: synth.Sine}}
	samples := synth.Render(glide, testPeakDB)
	want := synth.Hz(7 * 240.0 / 250.0)
	if got := pitchHz(samples[ms(235):ms(245)]); !approx.Equal(want, got, 2*pitchTolerance) {
		t.Errorf("pitch near end = %v, want %v", got, want)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `make test`
Expected: FAIL to build with `undefined: synth.Render` and similar.

- [ ] **Step 4: Write the implementation**

`synth/tone.go`:

```go
// Package synth renders Claude Code activity cues as audio sample buffers.
package synth

import (
	"math"
	"time"
)

// SampleRate is the output rate in frames per second.
const SampleRate = 48000

// TonicHz is the frequency of the tonic, D5.
const TonicHz = 587.33

const (
	semitonesPerOctave = 12
	decibelsPerDecade  = 20
	attack             = 3 * time.Millisecond
	release            = 5 * time.Millisecond
)

// Partial is one harmonic of a timbre with its relative amplitude.
type Partial struct {
	Harmonic  float64
	Amplitude float64
}

// Sine is a pure tone.
var Sine = []Partial{{Harmonic: 1, Amplitude: 1}}

// Buzzy adds odd harmonics for a rougher tone.
var Buzzy = []Partial{{Harmonic: 1, Amplitude: 1}, {Harmonic: 3, Amplitude: 0.3}, {Harmonic: 5, Amplitude: 0.15}}

// Note is one tone that glides from From to To semitones above the tonic.
type Note struct {
	From, To float64
	Onset    time.Duration
	Length   time.Duration
	Decay    time.Duration
	Timbre   []Partial
}

// Amplitude converts decibels relative to full scale to a linear amplitude.
func Amplitude(db float64) float64 { return math.Pow(10, db/decibelsPerDecade) }

// Hz returns the frequency of a pitch given in semitones above the tonic.
func Hz(semitones float64) float64 { return TonicHz * math.Exp2(semitones/semitonesPerOctave) }

// Semitones returns the pitch of a frequency in semitones above the tonic.
func Semitones(hz float64) float64 { return semitonesPerOctave * math.Log2(hz/TonicHz) }

// Render mixes a score into one buffer normalized to peak at peakDB.
func Render(score []Note, peakDB float64) []float32 {
	buf := make([]float64, end(score))
	for _, n := range score {
		addNote(buf[frames(n.Onset):], n)
	}
	return normalize(buf, Amplitude(peakDB))
}

func frames(d time.Duration) int { return int(d.Seconds() * SampleRate) }

func end(score []Note) int {
	last := 0
	for _, n := range score {
		last = max(last, frames(n.Onset)+frames(n.Length))
	}
	return last
}

func addNote(dst []float64, n Note) {
	count := frames(n.Length)
	phase := 0.0
	for i := range count {
		progress := float64(i) / float64(count)
		phase += 2 * math.Pi * Hz(n.From+(n.To-n.From)*progress) / SampleRate
		dst[i] += envelope(i, count, n.Decay) * timbre(phase, n.Timbre)
	}
}

func envelope(i, count int, decay time.Duration) float64 {
	rise := min(1, float64(i)/float64(frames(attack)))
	fall := min(1, float64(count-1-i)/float64(frames(release)))
	return rise * fall * math.Exp(-float64(i)/SampleRate/decay.Seconds())
}

func timbre(phase float64, partials []Partial) float64 {
	sum := 0.0
	for _, p := range partials {
		sum += p.Amplitude * math.Sin(p.Harmonic*phase)
	}
	return sum
}

func normalize(buf []float64, peak float64) []float32 {
	loudest := 0.0
	for _, x := range buf {
		loudest = max(loudest, math.Abs(x))
	}
	out := make([]float32, len(buf))
	for i, x := range buf {
		out[i] = float32(x * peak / loudest)
	}
	return out
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `make test`
Expected: PASS (`ok meknik/synth`).

- [ ] **Step 6: Lint and commit**

```bash
make lint && make test && fossil changes
fossil add go.mod Makefile mise.toml .golangci.yml internal/approx/approx.go synth/tone.go synth/tone_test.go
fossil commit -m "Add module scaffold and tone synthesis" go.mod Makefile mise.toml .golangci.yml internal/approx/approx.go synth/tone.go synth/tone_test.go
```

---

### Task 2: Cue definitions

**Files:**
- Create: `synth/cue.go`
- Test: `synth/cue_test.go`

**Interfaces:**
- Consumes: `synth.Render`, `synth.Note`, `synth.Sine`, `synth.Buzzy`, `synth.Semitones` (Task 1)
- Produces:
  - `type synth.Cue string` with constants `Tick, Spawn, Return, Done, Failed, Permission, Ask`
  - `synth.TickPeakDB = -24.0`, `synth.CuePeakDB = -18.0`, `synth.TickJitterDB = 3.0`
  - `synth.Sound(c Cue, rng *rand.Rand) ([]float32, bool)` (`math/rand/v2`; reports false for an unknown cue)

- [ ] **Step 1: Write the failing tests**

`synth/cue_test.go` (reuses `ms`, `peak`, `pitchHz`, `pitchTolerance`, `levelTolerance` from `tone_test.go`):

```go
package synth_test

import (
	"math/rand/v2"
	"testing"

	"meknik/internal/approx"
	"meknik/synth"
)

const (
	tickLowHz  = 1800.0
	tickHighHz = 2600.0
)

var allCues = []synth.Cue{synth.Tick, synth.Spawn, synth.Return, synth.Done, synth.Failed, synth.Permission, synth.Ask}

func newRNG() *rand.Rand { return rand.New(rand.NewPCG(1, 2)) }

func sound(t *testing.T, c synth.Cue) []float32 {
	t.Helper()
	samples, ok := synth.Sound(c, newRNG())
	if !ok {
		t.Fatalf("Sound(%q) unknown", c)
	}
	return samples
}

func TestCueDurations(t *testing.T) {
	want := map[synth.Cue]int{
		synth.Tick: ms(12), synth.Spawn: ms(80), synth.Return: ms(80), synth.Done: ms(380),
		synth.Failed: ms(300), synth.Permission: ms(370), synth.Ask: ms(250),
	}
	for c, frames := range want {
		t.Run(string(c), func(t *testing.T) {
			if got := len(sound(t, c)); got != frames {
				t.Errorf("len = %d, want %d", got, frames)
			}
		})
	}
}

func TestCuesStartSilent(t *testing.T) {
	for _, c := range allCues {
		t.Run(string(c), func(t *testing.T) {
			if got := sound(t, c)[0]; got != 0 {
				t.Errorf("first sample = %v, want 0", got)
			}
		})
	}
}

func TestCuesEndSilent(t *testing.T) {
	for _, c := range allCues {
		t.Run(string(c), func(t *testing.T) {
			samples := sound(t, c)
			if got := samples[len(samples)-1]; got != 0 {
				t.Errorf("last sample = %v, want 0", got)
			}
		})
	}
}

func TestCuesPeakAtCueLevel(t *testing.T) {
	for _, c := range allCues[1:] {
		t.Run(string(c), func(t *testing.T) {
			if got := peak(sound(t, c)); !approx.Equal(synth.Amplitude(synth.CuePeakDB), got, levelTolerance) {
				t.Errorf("peak = %v, want %v", got, synth.Amplitude(synth.CuePeakDB))
			}
		})
	}
}

func TestTickStaysUnderJitteredLevel(t *testing.T) {
	if got := peak(sound(t, synth.Tick)); got > synth.Amplitude(synth.TickPeakDB+synth.TickJitterDB) {
		t.Errorf("peak = %v, want <= %v", got, synth.Amplitude(synth.TickPeakDB+synth.TickJitterDB))
	}
}

func TestTickPitchInRange(t *testing.T) {
	if got := pitchHz(sound(t, synth.Tick)); got < tickLowHz*(1-pitchTolerance) || got > tickHighHz*(1+pitchTolerance) {
		t.Errorf("pitch = %v, want within [%v, %v]", got, tickLowHz, tickHighHz)
	}
}

func TestCueFinalPitch(t *testing.T) {
	cases := []struct {
		cue       synth.Cue
		fromMs    int
		toMs      int
		semitones float64
	}{
		{synth.Spawn, 45, 75, 19},
		{synth.Return, 45, 75, 12},
		{synth.Done, 300, 370, 0},
		{synth.Permission, 200, 360, 11},
	}
	for _, tc := range cases {
		t.Run(string(tc.cue), func(t *testing.T) {
			got := pitchHz(sound(t, tc.cue)[ms(tc.fromMs):ms(tc.toMs)])
			if !approx.Equal(synth.Hz(tc.semitones), got, pitchTolerance) {
				t.Errorf("pitch = %v, want %v", got, synth.Hz(tc.semitones))
			}
		})
	}
}

func TestUnknownCueIsRejected(t *testing.T) {
	if _, ok := synth.Sound("bogus", newRNG()); ok {
		t.Error("Sound(bogus) ok = true, want false")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `make test`
Expected: FAIL to build with `undefined: synth.Cue`.

- [ ] **Step 3: Write the implementation**

`synth/cue.go`:

```go
package synth

import (
	"math/rand/v2"
	"time"
)

// Cue names one kind of activity sound.
type Cue string

// The cues the daemon can play.
const (
	Tick       Cue = "tick"
	Spawn      Cue = "spawn"
	Return     Cue = "return"
	Done       Cue = "done"
	Failed     Cue = "failed"
	Permission Cue = "permission"
	Ask        Cue = "ask"
)

// Peak levels in dBFS, with the random level spread applied to ticks.
const (
	TickPeakDB   = -24.0
	CuePeakDB    = -18.0
	TickJitterDB = 3.0
)

const (
	tickLowHz  = 1800.0
	tickHighHz = 2600.0
	tickLength = 12 * time.Millisecond
	tickDecay  = 3 * time.Millisecond
	sustain    = time.Hour
	bloop      = 40 * time.Millisecond
	cadenceGap = 90 * time.Millisecond
	ring       = 200 * time.Millisecond
	ringDecay  = 80 * time.Millisecond
	buzzNote   = 150 * time.Millisecond
	knock      = 60 * time.Millisecond
	hang       = 250 * time.Millisecond
	glide      = 250 * time.Millisecond
)

var scores = map[Cue][]Note{
	Spawn:  {held(12, 0, bloop), held(19, bloop, bloop)},
	Return: {held(19, 0, bloop), held(12, bloop, bloop)},
	Done:   {rung(7, 0), rung(4, cadenceGap), rung(0, 2*cadenceGap)},
	Failed: {
		{From: -6, To: -6, Length: buzzNote, Decay: sustain, Timbre: Buzzy},
		{From: -12, To: -12, Onset: buzzNote, Length: buzzNote, Decay: sustain, Timbre: Buzzy},
	},
	Permission: {held(5, 0, knock), held(5, knock, knock), held(11, 2*knock, hang)},
	Ask:        {{From: 0, To: 7, Length: glide, Decay: sustain, Timbre: Sine}},
}

// Sound renders a cue, reporting false for an unknown cue.
func Sound(c Cue, rng *rand.Rand) ([]float32, bool) {
	if c == Tick {
		return tick(rng), true
	}
	score, ok := scores[c]
	if !ok {
		return nil, false
	}
	return Render(score, CuePeakDB), true
}

func tick(rng *rand.Rand) []float32 {
	pitch := Semitones(tickLowHz + rng.Float64()*(tickHighHz-tickLowHz))
	jitter := (2*rng.Float64() - 1) * TickJitterDB
	return Render([]Note{{From: pitch, To: pitch, Length: tickLength, Decay: tickDecay, Timbre: Sine}}, TickPeakDB+jitter)
}

func held(semitones float64, onset, length time.Duration) Note {
	return Note{From: semitones, To: semitones, Onset: onset, Length: length, Decay: sustain, Timbre: Sine}
}

func rung(semitones float64, onset time.Duration) Note {
	return Note{From: semitones, To: semitones, Onset: onset, Length: ring, Decay: ringDecay, Timbre: Sine}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `make test`
Expected: PASS.

- [ ] **Step 5: Lint and commit**

```bash
make lint && make test && fossil changes
fossil add synth/cue.go synth/cue_test.go
fossil commit -m "Add cue definitions" synth/cue.go synth/cue_test.go
```

---

### Task 3: Hook event mapping

**Files:**
- Create: `hookevent/hookevent.go`
- Test: `hookevent/hookevent_test.go`

**Interfaces:**
- Consumes: `synth.Cue` and its constants (Task 2)
- Produces: `hookevent.Parse(r io.Reader) (synth.Cue, bool, error)` (false means the event has no cue)

- [ ] **Step 1: Write the failing tests**

`hookevent/hookevent_test.go`:

```go
package hookevent_test

import (
	"strings"
	"testing"

	"meknik/hookevent"
	"meknik/synth"
)

type result struct {
	cue synth.Cue
	ok  bool
}

func TestParseMapsEvents(t *testing.T) {
	cases := map[string]result{
		`{"hook_event_name":"PreToolUse","tool_name":"Bash"}`:            {synth.Tick, true},
		`{"hook_event_name":"PreToolUse","tool_name":"AskUserQuestion"}`: {synth.Ask, true},
		`{"hook_event_name":"SubagentStart"}`:                            {synth.Spawn, true},
		`{"hook_event_name":"SubagentStop"}`:                             {synth.Return, true},
		`{"hook_event_name":"Stop"}`:                                     {synth.Done, true},
		`{"hook_event_name":"StopFailure"}`:                              {synth.Failed, true},
		`{"hook_event_name":"PermissionRequest","tool_name":"Bash"}`:     {synth.Permission, true},
		`{"hook_event_name":"PostToolUse","tool_name":"Bash"}`:           {"", false},
		`{"hook_event_name":"Notification"}`:                             {"", false},
	}
	for payload, want := range cases {
		t.Run(payload, func(t *testing.T) {
			cue, ok, _ := hookevent.Parse(strings.NewReader(payload))
			if got := (result{cue, ok}); got != want {
				t.Errorf("Parse = %+v, want %+v", got, want)
			}
		})
	}
}

func TestParseRejectsMalformedJSON(t *testing.T) {
	if _, _, err := hookevent.Parse(strings.NewReader("{")); err == nil {
		t.Error("Parse error = nil, want an error")
	}
}

func TestParseRejectsEmptyInput(t *testing.T) {
	if _, _, err := hookevent.Parse(strings.NewReader("")); err == nil {
		t.Error("Parse error = nil, want an error")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `make test`
Expected: FAIL to build with `undefined: hookevent.Parse`.

- [ ] **Step 3: Write the implementation**

`hookevent/hookevent.go`:

```go
// Package hookevent maps Claude Code hook payloads to cues.
package hookevent

import (
	"encoding/json"
	"io"

	"meknik/synth"
)

const (
	preToolUse  = "PreToolUse"
	askToolName = "AskUserQuestion"
)

var eventCues = map[string]synth.Cue{
	preToolUse:          synth.Tick,
	"SubagentStart":     synth.Spawn,
	"SubagentStop":      synth.Return,
	"Stop":              synth.Done,
	"StopFailure":       synth.Failed,
	"PermissionRequest": synth.Permission,
}

type payload struct {
	Event string `json:"hook_event_name"`
	Tool  string `json:"tool_name"`
}

// Parse reads a hook payload and returns its cue, reporting false when the event has none.
func Parse(r io.Reader) (synth.Cue, bool, error) {
	var p payload
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return "", false, err
	}
	if p.Event == preToolUse && p.Tool == askToolName {
		return synth.Ask, true, nil
	}
	c, ok := eventCues[p.Event]
	return c, ok, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `make test`
Expected: PASS.

- [ ] **Step 5: Lint and commit**

```bash
make lint && make test && fossil changes
fossil add hookevent/hookevent.go hookevent/hookevent_test.go
fossil commit -m "Map hook events to cues" hookevent/hookevent.go hookevent/hookevent_test.go
```

---

### Task 4: Mixer

**Files:**
- Create: `mixer/mixer.go`
- Test: `mixer/mixer_test.go`

**Interfaces:**
- Consumes: `approx.Equal` (Task 1, tests only)
- Produces:
  - `mixer.MaxVoices = 8`
  - `type mixer.Sound struct{ Samples []float32; Tick bool }`
  - `mixer.New(gain, ceiling float64) *Mixer` (both linear amplitudes)
  - `(*Mixer).Add(s Sound)`, `(*Mixer).Fill(block []float32)`, `(*Mixer).Voices() int`

- [ ] **Step 1: Write the failing tests**

`mixer/mixer_test.go`:

```go
package mixer_test

import (
	"testing"

	"meknik/internal/approx"
	"meknik/mixer"
)

const (
	unity        = 1.0
	wideCeiling  = 100.0
	tightCeiling = 0.25
	quiet        = 0.001
	saturating   = 0.1
	loud         = 0.5
	tolerance    = 1e-3
	blockSize    = 4
)

func constant(value float32, frames int, tick bool) mixer.Sound {
	samples := make([]float32, frames)
	for i := range samples {
		samples[i] = value
	}
	return mixer.Sound{Samples: samples, Tick: tick}
}

func firstFrame(m *mixer.Mixer) float64 {
	block := make([]float32, blockSize)
	m.Fill(block)
	return float64(block[0])
}

func fullPool(tick bool) *mixer.Mixer {
	m := mixer.New(unity, wideCeiling)
	for range mixer.MaxVoices {
		m.Add(constant(quiet, blockSize, tick))
	}
	return m
}

func TestFillSumsVoices(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.Add(constant(quiet, blockSize, true))
	m.Add(constant(quiet, blockSize, false))
	if got := firstFrame(m); !approx.Equal(2*quiet, got, tolerance) {
		t.Errorf("frame = %v, want %v", got, 2*quiet)
	}
}

func TestFillAppliesGain(t *testing.T) {
	m := mixer.New(2*unity, wideCeiling)
	m.Add(constant(quiet, blockSize, false))
	if got := firstFrame(m); !approx.Equal(2*quiet, got, tolerance) {
		t.Errorf("frame = %v, want %v", got, 2*quiet)
	}
}

func TestFillHoldsBelowCeiling(t *testing.T) {
	m := mixer.New(unity, tightCeiling)
	for range mixer.MaxVoices {
		m.Add(constant(saturating, blockSize, false))
	}
	if got := firstFrame(m); got >= tightCeiling {
		t.Errorf("frame = %v, want < %v", got, tightCeiling)
	}
}

func TestFinishedVoiceIsRetired(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.Add(constant(quiet, blockSize-1, false))
	m.Fill(make([]float32, blockSize))
	if got := m.Voices(); got != 0 {
		t.Errorf("Voices = %d, want 0", got)
	}
}

func TestUnfinishedVoiceContinuesInNextBlock(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.Add(constant(quiet, blockSize+1, false))
	m.Fill(make([]float32, blockSize))
	if got := firstFrame(m); !approx.Equal(quiet, got, tolerance) {
		t.Errorf("frame = %v, want %v", got, quiet)
	}
}

func TestFullPoolDropsTick(t *testing.T) {
	m := fullPool(true)
	m.Add(constant(loud, blockSize, true))
	if got := firstFrame(m); !approx.Equal(mixer.MaxVoices*quiet, got, tolerance) {
		t.Errorf("frame = %v, want %v", got, mixer.MaxVoices*quiet)
	}
}

func TestFullPoolCueReplacesTick(t *testing.T) {
	m := fullPool(true)
	m.Add(constant(loud, blockSize, false))
	want := (mixer.MaxVoices-1)*quiet + loud
	if got := firstFrame(m); !approx.Equal(want, got, tolerance) {
		t.Errorf("frame = %v, want %v", got, want)
	}
}

func TestFullPoolOfCuesDropsCue(t *testing.T) {
	m := fullPool(false)
	m.Add(constant(loud, blockSize, false))
	if got := firstFrame(m); !approx.Equal(mixer.MaxVoices*quiet, got, tolerance) {
		t.Errorf("frame = %v, want %v", got, mixer.MaxVoices*quiet)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `make test`
Expected: FAIL to build with `undefined: mixer.New`.

- [ ] **Step 3: Write the implementation**

`mixer/mixer.go`:

```go
// Package mixer sums active cue voices into output blocks under a soft limiter.
package mixer

import (
	"math"
	"slices"
)

// MaxVoices caps how many sounds play at once.
const MaxVoices = 8

// Sound is a rendered cue queued for playback.
type Sound struct {
	Samples []float32
	Tick    bool
}

type voice struct {
	Sound
	pos int
}

// Mixer holds the active voices and the output gain staging.
type Mixer struct {
	voices  []*voice
	gain    float64
	ceiling float64
}

// New returns a mixer that applies gain before a tanh limiter held below ceiling.
func New(gain, ceiling float64) *Mixer { return &Mixer{gain: gain, ceiling: ceiling} }

// Add starts a sound; when the pool is full a tick is dropped and a cue replaces the oldest tick.
func (m *Mixer) Add(s Sound) {
	if len(m.voices) < MaxVoices {
		m.voices = append(m.voices, &voice{Sound: s})
		return
	}
	if s.Tick {
		return
	}
	if i := slices.IndexFunc(m.voices, func(v *voice) bool { return v.Tick }); i >= 0 {
		m.voices = append(slices.Delete(m.voices, i, i+1), &voice{Sound: s})
	}
}

// Fill renders the next block and retires finished voices.
func (m *Mixer) Fill(block []float32) {
	clear(block)
	for _, v := range m.voices {
		n := min(len(block), len(v.Samples)-v.pos)
		for i := range n {
			block[i] += v.Samples[v.pos+i]
		}
		v.pos += n
	}
	m.voices = slices.DeleteFunc(m.voices, func(v *voice) bool { return v.pos == len(v.Samples) })
	for i, x := range block {
		block[i] = float32(m.ceiling * math.Tanh(m.gain*float64(x)/m.ceiling))
	}
}

// Voices reports how many sounds are still playing.
func (m *Mixer) Voices() int { return len(m.voices) }
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `make test`
Expected: PASS.

- [ ] **Step 5: Lint and commit**

```bash
make lint && make test && fossil changes
fossil add mixer/mixer.go mixer/mixer_test.go
fossil commit -m "Add voice mixer with drop policy and limiter" mixer/mixer.go mixer/mixer_test.go
```

---

### Task 5: Playback encoding and socket transport

**Files:**
- Create: `playback/playback.go`
- Test: `playback/playback_test.go`
- Create: `transport/transport.go`
- Test: `transport/transport_test.go`
- Modify: `go.mod`, `go.sum` (adds `golang.org/x/sys`)

**Interfaces:**
- Consumes: `synth.Cue` (Task 2), `synth.SampleRate` (Task 1)
- Produces:
  - `playback.Encode(dst []byte, samples []float32) []byte`
  - `playback.Start() (*Stream, error)`, with `(*Stream).Write([]byte) (int, error)` and `(*Stream).Close() error`
  - `transport.Listen(path string) (*net.UnixConn, error)`
  - `transport.Serve(conn *net.UnixConn, play func(synth.Cue)) error` (returns nil once the connection is closed)
  - `transport.Send(path string, c synth.Cue) error`
  - `transport.Deliver(path string, c synth.Cue, spawn func() error, timeout, interval time.Duration) error`

- [ ] **Step 1: Add the dependency**

```bash
cd /home/rc/meknik && go get golang.org/x/sys@latest
```

- [ ] **Step 2: Write the failing tests**

`playback/playback_test.go`:

```go
package playback_test

import (
	"bytes"
	"testing"

	"meknik/playback"
)

func TestEncodeWritesLittleEndianFloat32(t *testing.T) {
	want := []byte{0x00, 0x00, 0x80, 0x3f}
	if got := playback.Encode(nil, []float32{1}); !bytes.Equal(want, got) {
		t.Errorf("Encode(1) = %x, want %x", got, want)
	}
}

func TestEncodeAppendsToDestination(t *testing.T) {
	if got := len(playback.Encode(make([]byte, 2), []float32{0, 0})); got != 10 {
		t.Errorf("len = %d, want 10", got)
	}
}
```

`transport/transport_test.go`:

```go
package transport_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"meknik/synth"
	"meknik/transport"
)

const (
	socketName    = "meknik.sock"
	receiveWait   = time.Second
	shortTimeout  = 20 * time.Millisecond
	retryInterval = 5 * time.Millisecond
)

var errSpawn = errors.New("spawn failed")

func socketPath(t *testing.T) string { return filepath.Join(t.TempDir(), socketName) }

func serve(t *testing.T, path string) <-chan synth.Cue {
	t.Helper()
	conn, err := transport.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	cues := make(chan synth.Cue, 1)
	go func() { _ = transport.Serve(conn, func(c synth.Cue) { cues <- c }) }()
	return cues
}

func receive(cues <-chan synth.Cue) synth.Cue {
	select {
	case c := <-cues:
		return c
	case <-time.After(receiveWait):
		return ""
	}
}

func noSpawn() error { return nil }

func TestSendReachesServer(t *testing.T) {
	path := socketPath(t)
	cues := serve(t, path)
	_ = transport.Send(path, synth.Done)
	if got := receive(cues); got != synth.Done {
		t.Errorf("received %q, want %q", got, synth.Done)
	}
}

func TestSendWithoutDaemonFails(t *testing.T) {
	if err := transport.Send(socketPath(t), synth.Done); err == nil {
		t.Error("Send error = nil, want an error")
	}
}

func TestListenReplacesStaleSocket(t *testing.T) {
	path := socketPath(t)
	_ = os.WriteFile(path, nil, 0o600)
	cues := serve(t, path)
	_ = transport.Send(path, synth.Tick)
	if got := receive(cues); got != synth.Tick {
		t.Errorf("received %q, want %q", got, synth.Tick)
	}
}

func TestServeReturnsNilWhenClosed(t *testing.T) {
	conn, _ := transport.Listen(socketPath(t))
	_ = conn.Close()
	if err := transport.Serve(conn, func(synth.Cue) {}); err != nil {
		t.Errorf("Serve error = %v, want nil", err)
	}
}

func TestDeliverSkipsSpawnWhenDaemonRuns(t *testing.T) {
	path := socketPath(t)
	serve(t, path)
	spawned := false
	_ = transport.Deliver(path, synth.Tick, func() error { spawned = true; return nil }, shortTimeout, retryInterval)
	if spawned {
		t.Error("spawned = true, want false")
	}
}

func TestDeliverSpawnsDaemonAndSends(t *testing.T) {
	path := socketPath(t)
	var cues <-chan synth.Cue
	_ = transport.Deliver(path, synth.Done, func() error { cues = serve(t, path); return nil }, shortTimeout, retryInterval)
	if got := receive(cues); got != synth.Done {
		t.Errorf("received %q, want %q", got, synth.Done)
	}
}

func TestDeliverReportsSpawnFailure(t *testing.T) {
	err := transport.Deliver(socketPath(t), synth.Done, func() error { return errSpawn }, shortTimeout, retryInterval)
	if !errors.Is(err, errSpawn) {
		t.Errorf("Deliver error = %v, want %v", err, errSpawn)
	}
}

func TestDeliverTimesOutWhenDaemonNeverAppears(t *testing.T) {
	if err := transport.Deliver(socketPath(t), synth.Done, noSpawn, shortTimeout, retryInterval); err == nil {
		t.Error("Deliver error = nil, want an error")
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `make test`
Expected: FAIL to build with `undefined: playback.Encode` and `undefined: transport.Listen`.

- [ ] **Step 4: Write the implementation**

`playback/playback.go`:

```go
// Package playback streams float32 PCM to PipeWire through pw-cat.
package playback

import (
	"encoding/binary"
	"errors"
	"math"
	"os"
	"os/exec"
	"strconv"

	"golang.org/x/sys/unix"

	"meknik/synth"
)

// PipeBytes bounds the pipe to pw-cat so that queued audio stays short.
const PipeBytes = 4096

var pwCatArgs = []string{
	"--playback", "--raw",
	"--format", "f32",
	"--rate", strconv.Itoa(synth.SampleRate),
	"--channels", "1",
	"--properties", "application.name=meknik",
	"-",
}

// Encode appends samples to dst as little-endian float32 bytes.
func Encode(dst []byte, samples []float32) []byte {
	for _, x := range samples {
		dst = binary.LittleEndian.AppendUint32(dst, math.Float32bits(x))
	}
	return dst
}

// Stream is a running pw-cat fed through a short pipe.
type Stream struct {
	cmd  *exec.Cmd
	pipe *os.File
}

// Start launches pw-cat reading raw PCM from a short pipe.
func Start() (*Stream, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	if _, err := unix.FcntlInt(w.Fd(), unix.F_SETPIPE_SZ, PipeBytes); err != nil {
		return nil, err
	}
	cmd := exec.Command("pw-cat", pwCatArgs...)
	cmd.Stdin = r
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Stream{cmd: cmd, pipe: w}, r.Close()
}

// Write sends encoded PCM to pw-cat.
func (s *Stream) Write(p []byte) (int, error) { return s.pipe.Write(p) }

// Close ends the stream and waits for pw-cat to drain and exit.
func (s *Stream) Close() error { return errors.Join(s.pipe.Close(), s.cmd.Wait()) }
```

`transport/transport.go`:

```go
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
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `make test`
Expected: PASS.

- [ ] **Step 6: Lint and commit**

```bash
make lint && make test && fossil changes
fossil add go.sum playback/playback.go playback/playback_test.go transport/transport.go transport/transport_test.go
fossil commit -m "Add pw-cat playback and socket transport" go.mod go.sum playback/playback.go playback/playback_test.go transport/transport.go transport/transport_test.go
```

---

### Task 6: Daemon lock and audio loop

**Files:**
- Create: `daemon/daemon.go`
- Test: `daemon/daemon_test.go`

**Interfaces:**
- Consumes: `mixer.Mixer`, `mixer.Sound`, `mixer.New` (Task 4), `playback.Encode` (Task 5)
- Produces:
  - `daemon.BlockFrames = 256`
  - `daemon.Lock(path string) (*os.File, bool, error)` (false means another daemon holds the lock)
  - `daemon.Play(m *mixer.Mixer, sounds <-chan mixer.Sound, out io.Writer, idle time.Duration, now func() time.Time) error`

- [ ] **Step 1: Write the failing tests**

`daemon/daemon_test.go`:

```go
package daemon_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"path/filepath"
	"testing"
	"time"

	"meknik/daemon"
	"meknik/internal/approx"
	"meknik/mixer"
)

const (
	lockName       = "meknik.lock"
	idle           = 10 * time.Millisecond
	clockStep      = time.Millisecond
	unity          = 1.0
	wideCeiling    = 100.0
	sample         = 0.1
	tolerance      = 1e-4
	bytesPerSample = 4
)

var errBroken = errors.New("pipe broken")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errBroken }

func steppingClock() func() time.Time {
	t := time.Unix(0, 0)
	return func() time.Time {
		t = t.Add(clockStep)
		return t
	}
}

func play(sounds <-chan mixer.Sound) *bytes.Buffer {
	var out bytes.Buffer
	_ = daemon.Play(mixer.New(unity, wideCeiling), sounds, &out, idle, steppingClock())
	return &out
}

func TestLockExcludesSecondHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), lockName)
	first, _, _ := daemon.Lock(path)
	t.Cleanup(func() { _ = first.Close() })
	if _, held, _ := daemon.Lock(path); held {
		t.Error("second Lock held = true, want false")
	}
}

func TestLockIsFreeAfterRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), lockName)
	first, _, _ := daemon.Lock(path)
	_ = first.Close()
	second, held, _ := daemon.Lock(path)
	t.Cleanup(func() { _ = second.Close() })
	if !held {
		t.Error("Lock after release held = false, want true")
	}
}

func TestPlayStopsWhenIdle(t *testing.T) {
	err := daemon.Play(mixer.New(unity, wideCeiling), make(chan mixer.Sound), &bytes.Buffer{}, idle, steppingClock())
	if err != nil {
		t.Errorf("Play error = %v, want nil", err)
	}
}

func TestPlayWritesWholeBlocks(t *testing.T) {
	if got := play(make(chan mixer.Sound)).Len() % (daemon.BlockFrames * bytesPerSample); got != 0 {
		t.Errorf("output length remainder = %d, want 0", got)
	}
}

func TestPlayWritesQueuedSound(t *testing.T) {
	sounds := make(chan mixer.Sound, 1)
	sounds <- mixer.Sound{Samples: []float32{sample}}
	got := float64(math.Float32frombits(binary.LittleEndian.Uint32(play(sounds).Bytes())))
	if !approx.Equal(sample, got, tolerance) {
		t.Errorf("first sample = %v, want %v", got, sample)
	}
}

func TestPlayReportsWriteFailure(t *testing.T) {
	err := daemon.Play(mixer.New(unity, wideCeiling), make(chan mixer.Sound), failingWriter{}, idle, steppingClock())
	if !errors.Is(err, errBroken) {
		t.Errorf("Play error = %v, want %v", err, errBroken)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `make test`
Expected: FAIL to build with `undefined: daemon.Lock`.

- [ ] **Step 3: Write the implementation**

`daemon/daemon.go`:

```go
// Package daemon runs the single-instance audio loop that plays cues.
package daemon

import (
	"errors"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"meknik/mixer"
	"meknik/playback"
)

// BlockFrames is the number of frames rendered per write.
const BlockFrames = 256

const lockMode = 0o600

// Lock takes the single-instance lock, reporting false when another daemon holds it.
func Lock(path string) (*os.File, bool, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, lockMode)
	if err != nil {
		return nil, false, err
	}
	err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return nil, false, f.Close()
	}
	if err != nil {
		return nil, false, errors.Join(err, f.Close())
	}
	return f, true, nil
}

// Play mixes queued sounds into out block by block until idle passes with no new sound.
func Play(m *mixer.Mixer, sounds <-chan mixer.Sound, out io.Writer, idle time.Duration, now func() time.Time) error {
	block := make([]float32, BlockFrames)
	var pcm []byte
	last := now()
	for now().Sub(last) < idle {
		last = drain(m, sounds, last, now)
		m.Fill(block)
		pcm = playback.Encode(pcm[:0], block)
		if _, err := out.Write(pcm); err != nil {
			return err
		}
	}
	return nil
}

func drain(m *mixer.Mixer, sounds <-chan mixer.Sound, last time.Time, now func() time.Time) time.Time {
	for {
		select {
		case s := <-sounds:
			m.Add(s)
			last = now()
		default:
			return last
		}
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `make test`
Expected: PASS.

- [ ] **Step 5: Lint and commit**

```bash
make lint && make test && fossil changes
fossil add daemon/daemon.go daemon/daemon_test.go
fossil commit -m "Add daemon lock and audio loop" daemon/daemon.go daemon/daemon_test.go
```

---

### Task 7: Command wiring and manual audio check

**Files:**
- Create: `cmd/meknik/main.go`
- Test: `cmd/meknik/main_test.go`

**Interfaces:**
- Consumes: everything above.
- Produces: the `meknik` binary with subcommands `hook`, `daemon` and `play <cue>`.

- [ ] **Step 1: Write the failing tests**

`cmd/meknik/main_test.go`:

```go
package main

import (
	"errors"
	"testing"

	"meknik/internal/approx"
	"meknik/synth"
)

const tolerance = 1e-9

func TestParseGainDefaultsToUnity(t *testing.T) {
	if got, _ := parseGain(""); got != 1 {
		t.Errorf("parseGain(\"\") = %v, want 1", got)
	}
}

func TestParseGainConvertsDecibels(t *testing.T) {
	if got, _ := parseGain("-6"); !approx.Equal(synth.Amplitude(-6), got, tolerance) {
		t.Errorf("parseGain(-6) = %v, want %v", got, synth.Amplitude(-6))
	}
}

func TestParseGainRejectsGarbage(t *testing.T) {
	if _, err := parseGain("loud"); err == nil {
		t.Error("parseGain(loud) error = nil, want an error")
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	if err := run([]string{"bogus"}); !errors.Is(err, errUsage) {
		t.Errorf("run error = %v, want %v", err, errUsage)
	}
}

func TestPlayRejectsUnknownCue(t *testing.T) {
	if err := run([]string{"play", "bogus"}); err == nil {
		t.Error("play bogus error = nil, want an error")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `make test`
Expected: FAIL to build with `undefined: parseGain`.

- [ ] **Step 3: Write the implementation**

`cmd/meknik/main.go`:

```go
// Command meknik plays synthesized audio cues for Claude Code hook events.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"syscall"
	"time"

	"meknik/daemon"
	"meknik/hookevent"
	"meknik/mixer"
	"meknik/playback"
	"meknik/synth"
	"meknik/transport"
)

const (
	appName          = "meknik"
	socketName       = "meknik.sock"
	lockName         = "meknik.lock"
	logName          = "meknik.log"
	gainEnv          = "MEKNIK_GAIN"
	spawnTimeout     = time.Second
	retryInterval    = 20 * time.Millisecond
	idleTimeout      = 10 * time.Minute
	limiterCeilingDB = -12.0
	soundQueue       = 64
	logMode          = 0o644
	dirMode          = 0o755
)

var errUsage = errors.New("usage: meknik hook | meknik daemon | meknik play <cue>")

func main() {
	logFile, err := openLog()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, nil)))
	if err := run(os.Args[1:]); err != nil {
		slog.Error("meknik failed", "args", os.Args[1:], "err", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	switch {
	case slices.Equal(args, []string{"hook"}):
		return runHook()
	case slices.Equal(args, []string{"daemon"}):
		return runDaemon()
	case len(args) == 2 && args[0] == "play":
		return runPlay(synth.Cue(args[1]))
	}
	return errUsage
}

func runHook() error {
	cue, ok, err := hookevent.Parse(os.Stdin)
	if err != nil || !ok {
		return err
	}
	return transport.Deliver(runtimePath(socketName), cue, spawnDaemon, spawnTimeout, retryInterval)
}

func spawnDaemon() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(self, "daemon")
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func runDaemon() error {
	lock, held, err := daemon.Lock(runtimePath(lockName))
	if err != nil || !held {
		return err
	}
	defer lock.Close()
	gain, err := parseGain(os.Getenv(gainEnv))
	if err != nil {
		return err
	}
	socket := runtimePath(socketName)
	conn, err := transport.Listen(socket)
	if err != nil {
		return err
	}
	stream, err := playback.Start()
	if err != nil {
		return errors.Join(err, conn.Close(), os.Remove(socket))
	}
	sounds := make(chan mixer.Sound, soundQueue)
	go func() {
		if err := transport.Serve(conn, enqueue(sounds, newRNG())); err != nil {
			slog.Error("socket failed", "err", err)
		}
	}()
	slog.Info("daemon started", "socket", socket, "gain", gain)
	playErr := daemon.Play(mixer.New(gain, synth.Amplitude(limiterCeilingDB)), sounds, stream, idleTimeout, time.Now)
	slog.Info("daemon stopping")
	return errors.Join(playErr, conn.Close(), os.Remove(socket), stream.Close())
}

func enqueue(sounds chan<- mixer.Sound, rng *rand.Rand) func(synth.Cue) {
	return func(c synth.Cue) {
		samples, ok := synth.Sound(c, rng)
		if !ok {
			slog.Warn("unknown cue", "cue", c)
			return
		}
		sounds <- mixer.Sound{Samples: samples, Tick: c == synth.Tick}
	}
}

func runPlay(c synth.Cue) error {
	samples, ok := synth.Sound(c, newRNG())
	if !ok {
		return fmt.Errorf("unknown cue %q", c)
	}
	stream, err := playback.Start()
	if err != nil {
		return err
	}
	_, err = stream.Write(playback.Encode(nil, samples))
	return errors.Join(err, stream.Close())
}

func parseGain(db string) (float64, error) {
	if db == "" {
		return 1, nil
	}
	v, err := strconv.ParseFloat(db, 64)
	return synth.Amplitude(v), err
}

func newRNG() *rand.Rand { return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())) }

func runtimePath(name string) string { return filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), name) }

func openLog() (*os.File, error) {
	dir, err := stateDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(dir, logName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, logMode)
}

func stateDir() (string, error) {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, appName), nil
	}
	home, err := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", appName), err
}
```

If `errcheck` flags `defer lock.Close()`, change it to `defer func() { _ = lock.Close() }()`. Don't add a `//nolint`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `make test`
Expected: PASS.

- [ ] **Step 5: Install and check by ear (manual; the user listens)**

```bash
make install
for c in tick spawn return done failed permission ask; do echo "$c"; ~/.local/bin/meknik play "$c"; sleep 0.5; done
```

Expected: seven distinct, quiet sounds. Ask the user to confirm they heard each one and that the levels are subtle.

Then check that the daemon autostarts:

```bash
echo '{"hook_event_name":"Stop"}' | ~/.local/bin/meknik hook; echo "exit=$?"
pgrep -a meknik
pactl list sink-inputs | grep -i 'application.name = "meknik"'
for i in $(seq 20); do echo '{"hook_event_name":"PreToolUse","tool_name":"Bash"}' | ~/.local/bin/meknik hook & done; wait
pgrep -c -f 'meknik daemon'
tail -n 5 ~/.local/state/meknik/meknik.log
```

Expected: `exit=0`. A single `meknik daemon` process is running (`pgrep -c` prints `1`). The stream appears with `application.name = "meknik"`. The burst of twenty ticks sounds like a patter. The log shows one `daemon started` and no errors.

- [ ] **Step 6: Lint and commit**

```bash
make lint && make test && fossil changes
fossil add cmd/meknik/main.go cmd/meknik/main_test.go
fossil commit -m "Wire meknik hook, daemon, and play commands" cmd/meknik/main.go cmd/meknik/main_test.go
```

---

### Task 8: Register the hooks

**Files:**
- Modify: `~/.claude/settings.json` (outside the repository, so there's no commit)

**Interfaces:**
- Consumes: `~/.local/bin/meknik hook` (Task 7)

- [ ] **Step 1: Get the user's confirmation before editing global settings**

Show the user the JSON below and wait for a yes.

- [ ] **Step 2: Back up and merge**

```bash
cp ~/.claude/settings.json ~/.claude/settings.json.pre-meknik
jq --arg cmd "$HOME/.local/bin/meknik hook" '
  def entry(m): [ (if m then {matcher: m} else {} end) + {hooks: [{type: "command", command: $cmd, async: true}]} ];
  .hooks.PreToolUse        += entry("*")
| .hooks.PermissionRequest += entry("*")
| .hooks.SubagentStart     += entry(null)
| .hooks.SubagentStop      += entry(null)
| .hooks.Stop              += entry(null)
| .hooks.StopFailure       += entry(null)
' ~/.claude/settings.json.pre-meknik > ~/.claude/settings.json
jq '.hooks | with_entries(select(.value[].hooks[].command | test("meknik")))' ~/.claude/settings.json
```

Expected: the final `jq` prints six events, each with one async `meknik hook` command.

- [ ] **Step 3: Verify live**

Run any tool call in a fresh Claude Code session, then:

```bash
tail -n 3 ~/.local/state/meknik/meknik.log; pgrep -a meknik
```

Expected: the daemon is running and there are no errors in the log. The user confirms they hear ticks during tool use and `done` at the end of the turn.
