# meknik rhythmic grid Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The daemon starts every sound on a 120 bpm grid (16th notes, 32nds under load), one sound per grid line, with a 4-sound queue that drops the newest when full; lamp events fire when a sound actually starts.

**Architecture:** A GTK-free `grid.Scheduler` does all timing in frames. `mixer` gains frame-offset starts and a `Cue` field on `Sound`. `daemon.Play` feeds arrivals to the scheduler and starts due sounds each block, reporting them through a callback that the daemon wiring uses to publish cue events.

**Tech Stack:** Go 1.26 in the podman build image, Fossil.

**Spec:** `docs/superpowers/specs/2026-10-08-meknik-grid-design.md`

## Global Constraints

- 120 bpm; beat = `synth.SampleRate * 60 / 120` = 24 000 frames; 16th = 6000 frames; 32nd = 3000 frames; quiet stretch = one bar = 4 beats = 96 000 frames.
- Grid lines are multiples of the slot length on a frame clock starting at 0. One sound per line: start = first line ≥ max(now, previous start + slot).
- FIFO queue of at most 4, ticks included; a sound arriving while 4 wait is dropped (newest dropped).
- An accepted sound resets the drop streak; the third drop in a row switches to 32nds; never finer than 32nds; one quiet bar (no arrivals) while at 32nds returns to 16ths.
- Cue events (`events.CueLine`) are published when a sound starts, never on arrival; dropped sounds publish nothing.
- `mixer.Sound.Cue synth.Cue` replaces `mixer.Sound.Tick`.
- Test: `make test`. Lint: `make lint`. Both pass before each commit.
- Fossil: one-line commits, no trailers, each path listed; `fossil add` new files. Never git; never `fossil revert`/`fossil rm`.
- Code style per `~/.claude/CLAUDE.md`: no comments beyond one-line doc comments and package headers; one assertion per test; tests < 5 s; named constants; no defensive checks; no fallbacks; DRY; `slog` for logging.

## Review Focus

1. **Slot change while sounds wait**: switching 16th→32nd or back mid-queue must not start two sounds on one line or skip phase. Pinned partly by `TestThirdDropInARowSwitchesToThirtySeconds`; the Task 2 reviewer traces `nextLine` across a switch.
2. **Quiet stretch measured from the last arrival, drops included**: a stream of dropped sounds is not quiet. Checked by the Task 2 reviewer.
3. **Mixer voice cap with delayed voices**: a voice added with an offset counts toward `MaxVoices` before it sounds; tick eviction still prefers ticks. Checked by the Task 1 reviewer.
4. **Lamps vs sound**: a lamp flashes at the block in which its sound starts (≤ 5.3 ms early within the block), never for a dropped sound. Checked by hand after Task 3.
5. **Daemon idle timer**: idle is measured from the last arrival; queued sounds still play out. Checked by the Task 3 reviewer.

---

## File Structure

| Path | Responsibility |
|---|---|
| `mixer/mixer.go`, `mixer/mixer_test.go` | `Sound.Cue`, `AddAt`, offset-aware `Fill`. Modified. |
| `grid/grid.go`, `grid/grid_test.go` | `Scheduler`, `Start`, `New`, `Offer`, `Due`. New. |
| `daemon/daemon.go`, `daemon/daemon_test.go` | `Play` uses the scheduler and a `started` callback. Modified. |
| `cmd/meknik/main.go`, `cmd/meknik/main_test.go` | `Cue` field; `announce`; wiring. Modified. |

---

### Task 1: Mixer — `Sound.Cue` and `AddAt`

**Files:** Modify `mixer/mixer.go`, `mixer/mixer_test.go`, `cmd/meknik/main.go` (the `mixer.Sound{...}` literal only).

**Interfaces — Produces:** `mixer.Sound{Samples []float32; Cue synth.Cue}`; `func (m *Mixer) AddAt(s Sound, offset int)`; `Add(s)` = `AddAt(s, 0)`.

- [ ] **Step 1: Write the failing tests.** In `mixer/mixer_test.go`, change the `constant` helper to build `mixer.Sound{Samples: samples, Cue: cue}` where `cue` is `synth.Tick` when `tick` is true and `synth.Done` otherwise (add `"meknik/synth"` import). Append:

```go
func TestAddAtDelaysFirstSample(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.AddAt(constant(loud, 1, false), 2)
	block := make([]float32, blockSize)
	m.Fill(block)
	if !approx.Equal(loud, float64(block[2]), tolerance) {
		t.Errorf("block[2] = %v, want %v", block[2], loud)
	}
}

func TestAddAtLeavesEarlierFramesSilent(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.AddAt(constant(loud, 1, false), 2)
	block := make([]float32, blockSize)
	m.Fill(block)
	if block[1] != 0 {
		t.Errorf("block[1] = %v, want 0", block[1])
	}
}

func TestDelayedVoiceContinuesInNextBlock(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.AddAt(constant(loud, blockSize, false), blockSize-1)
	m.Fill(make([]float32, blockSize))
	if got := firstFrame(m); !approx.Equal(loud, got, tolerance) {
		t.Errorf("next block first frame = %v, want %v", got, loud)
	}
}
```

- [ ] **Step 2: Run** `make test`; Expected: FAIL (unknown field `Cue`, undefined `AddAt`).

- [ ] **Step 3: Implement** in `mixer/mixer.go`:

```go
// Sound is a rendered cue queued for playback.
type Sound struct {
	Samples []float32
	Cue     synth.Cue
}

func (s Sound) tick() bool { return s.Cue == synth.Tick }
```

`Add` becomes `func (m *Mixer) Add(s Sound) { m.AddAt(s, 0) }`. `AddAt` holds the old `Add` body with `&voice{Sound: s, pos: -offset}` in both appends and `s.tick()`/`v.tick()` in place of `.Tick`; its doc comment: `// AddAt starts a sound offset frames into the next block; when the pool is full a tick is dropped and a cue replaces the oldest tick.` `Fill`:

```go
func (m *Mixer) Fill(block []float32) {
	clear(block)
	for _, v := range m.voices {
		for i := range block {
			if j := v.pos + i; j >= 0 && j < len(v.Samples) {
				block[i] += v.Samples[j]
			}
		}
		v.pos += len(block)
	}
	m.voices = slices.DeleteFunc(m.voices, func(v *voice) bool { return v.pos >= len(v.Samples) })
	for i, x := range block {
		block[i] = float32(m.gain * m.ceiling * math.Tanh(float64(x)/m.ceiling))
	}
}
```

In `cmd/meknik/main.go`, `mixer.Sound{Samples: samples, Tick: c == synth.Tick}` becomes `mixer.Sound{Samples: samples, Cue: c}`.

- [ ] **Step 4:** `make test` all ok; `make lint` `0 issues.`
- [ ] **Step 5: Commit**

```bash
fossil changes
fossil commit -m "Let the mixer start a sound at a frame offset and name its cue" mixer/mixer.go mixer/mixer_test.go cmd/meknik/main.go
```

---

### Task 2: `grid.Scheduler`

**Files:** Create `grid/grid.go`, `grid/grid_test.go`.

**Interfaces — Consumes:** `mixer.Sound`, `synth.SampleRate`. **Produces:** `grid.New() *Scheduler`; `(*Scheduler).Offer(mixer.Sound) bool`; `(*Scheduler).Due(frames int) []grid.Start`; `grid.Start{Sound mixer.Sound; Offset int}`.

- [ ] **Step 1: Write the failing tests** — `grid/grid_test.go`

```go
package grid_test

import (
	"testing"

	"meknik/grid"
	"meknik/mixer"
	"meknik/synth"
)

const (
	block        = 256
	sixteenth    = 6000
	thirtySecond = 3000
	bar          = 96000
	queueLimit   = 4
)

func sound() mixer.Sound { return mixer.Sound{Cue: synth.Done} }

func offers(s *grid.Scheduler, n int) {
	for range n {
		s.Offer(sound())
	}
}

func starts(s *grid.Scheduler, frames int) []int {
	var at []int
	for clock := 0; clock < frames; clock += block {
		for _, st := range s.Due(block) {
			at = append(at, clock+st.Offset)
		}
	}
	return at
}

func TestFirstSoundStartsAtFrameZero(t *testing.T) {
	s := grid.New()
	offers(s, 1)
	if got := s.Due(block); len(got) != 1 || got[0].Offset != 0 {
		t.Errorf("Due = %+v, want one start at offset 0", got)
	}
}

func TestSecondSoundStartsOneSixteenthLater(t *testing.T) {
	s := grid.New()
	offers(s, 2)
	if got := starts(s, 2*sixteenth); len(got) != 2 || got[1] != sixteenth {
		t.Errorf("starts = %v, want [0 %d]", got, sixteenth)
	}
}

func TestSoundBetweenLinesWaitsForNextLine(t *testing.T) {
	s := grid.New()
	s.Due(block)
	offers(s, 1)
	if got := starts(s, sixteenth); len(got) != 1 || got[0] != sixteenth-block {
		t.Errorf("starts = %v, want [%d] (relative to frame %d)", got, sixteenth-block, block)
	}
}

func TestFifthWaitingSoundIsDropped(t *testing.T) {
	s := grid.New()
	offers(s, queueLimit)
	if s.Offer(sound()) {
		t.Error("Offer accepted a fifth waiting sound")
	}
}

func TestAcceptanceResetsTheDropStreak(t *testing.T) {
	s := grid.New()
	offers(s, queueLimit+2)
	s.Due(block)
	offers(s, 2)
	if got := starts(s, 5*sixteenth); len(got) != queueLimit || got[1]-got[0] != sixteenth {
		t.Errorf("starts = %v, want 16th spacing", got)
	}
}

func TestThirdDropInARowSwitchesToThirtySeconds(t *testing.T) {
	s := grid.New()
	offers(s, queueLimit+3)
	if got := starts(s, 2*thirtySecond); len(got) < 2 || got[1] != thirtySecond {
		t.Errorf("starts = %v, want [0 %d]", got, thirtySecond)
	}
}

func TestQuietBarReturnsToSixteenths(t *testing.T) {
	s := grid.New()
	offers(s, queueLimit+3)
	starts(s, bar+queueLimit*thirtySecond)
	offers(s, 2)
	if got := starts(s, 2*sixteenth); len(got) != 2 || got[1]-got[0] != sixteenth {
		t.Errorf("starts = %v, want 16th spacing", got)
	}
}

func TestSoundDueAfterTheBlockIsNotReturned(t *testing.T) {
	s := grid.New()
	offers(s, 2)
	s.Due(block)
	if got := s.Due(block); len(got) != 0 {
		t.Errorf("Due = %+v, want nothing before the next line", got)
	}
}
```

(`TestAcceptanceResetsTheDropStreak`: 6 offers = 4 accepted + 2 drops; after one block a slot opens; 2 more offers = 1 accepted (streak reset) + 1 drop, so the streak never reaches 3 and spacing stays 16ths.)

- [ ] **Step 2: Run** `make test`; Expected: FAIL (`meknik/grid` has no non-test Go files).

- [ ] **Step 3: Implement** — `grid/grid.go`

```go
// Package grid quantizes sound starts to a rhythmic grid with a short queue.
package grid

import (
	"meknik/mixer"
	"meknik/synth"
)

const (
	beatsPerMinute    = 120
	secondsPerMinute  = 60
	beatsPerBar       = 4
	framesPerBeat     = synth.SampleRate * secondsPerMinute / beatsPerMinute
	sixteenth         = framesPerBeat / 4
	thirtySecond      = framesPerBeat / 8
	quietFrames       = beatsPerBar * framesPerBeat
	queueLimit        = 4
	dropsBeforeFaster = 2
)

// Start is a sound due to begin offset frames into the current block.
type Start struct {
	Sound  mixer.Sound
	Offset int
}

// Scheduler queues sounds and starts at most one per grid line.
type Scheduler struct {
	pending     []mixer.Sound
	slot        int
	clock       int
	free        int
	drops       int
	lastArrival int
}

// New returns a scheduler on a 16th-note grid at frame 0.
func New() *Scheduler { return &Scheduler{slot: sixteenth} }

// Offer queues a sound, reporting false when the queue is full and the sound is dropped.
func (s *Scheduler) Offer(snd mixer.Sound) bool {
	s.lastArrival = s.clock
	if len(s.pending) == queueLimit {
		s.drops++
		if s.drops > dropsBeforeFaster {
			s.slot = thirtySecond
		}
		return false
	}
	s.drops = 0
	s.pending = append(s.pending, snd)
	return true
}

// Due returns the sounds that start within the next frames frames and advances the clock.
func (s *Scheduler) Due(frames int) []Start {
	if s.slot == thirtySecond && s.clock-s.lastArrival >= quietFrames {
		s.slot = sixteenth
	}
	var due []Start
	for len(s.pending) > 0 {
		line := s.nextLine()
		if line >= s.clock+frames {
			break
		}
		due = append(due, Start{Sound: s.pending[0], Offset: line - s.clock})
		s.pending = s.pending[1:]
		s.free = line + s.slot
	}
	s.clock += frames
	return due
}

func (s *Scheduler) nextLine() int {
	from := max(s.clock, s.free)
	return (from + s.slot - 1) / s.slot * s.slot
}
```

- [ ] **Step 4:** `make test` all ok; `make lint` `0 issues.`
- [ ] **Step 5: Commit**

```bash
fossil add grid/grid.go grid/grid_test.go
fossil changes
fossil commit -m "Add the grid scheduler that quantizes sounds to 16th and 32nd notes" grid/grid.go grid/grid_test.go
```

---

### Task 3: Daemon plays on the grid; events at start

**Files:** Modify `daemon/daemon.go`, `daemon/daemon_test.go`, `cmd/meknik/main.go`, `cmd/meknik/main_test.go`.

**Interfaces — Consumes:** `grid.New`, `Offer`, `Due`, `mixer.AddAt`, `events.CueLine`. **Produces:** `daemon.Play(m *mixer.Mixer, s *grid.Scheduler, sounds <-chan mixer.Sound, gains <-chan float64, out io.Writer, idle time.Duration, now func() time.Time, started func(mixer.Sound)) error`; in `cmd/meknik`: `enqueue(sounds chan<- mixer.Sound, rng *rand.Rand) func(synth.Cue)` (no publish) and `announce(publish func(string)) func(mixer.Sound)`.

- [ ] **Step 1: Write the failing tests**

`daemon/daemon_test.go`: every `daemon.Play(mixer.New(unity, wideCeiling), X, Y, out, idle, clock)` call becomes `daemon.Play(mixer.New(unity, wideCeiling), grid.New(), X, Y, out, idle, clock, func(mixer.Sound) {})` (import `"meknik/grid"`). Append:

```go
func TestPlayReportsStartedSound(t *testing.T) {
	sounds := make(chan mixer.Sound, 1)
	sounds <- mixer.Sound{Samples: []float32{sample}, Cue: synth.Done}
	var started []synth.Cue
	_ = daemon.Play(mixer.New(unity, wideCeiling), grid.New(), sounds, make(chan float64), &bytes.Buffer{}, idle, steppingClock(),
		func(s mixer.Sound) { started = append(started, s.Cue) })
	if want := []synth.Cue{synth.Done}; !slices.Equal(started, want) {
		t.Errorf("started = %v, want %v", started, want)
	}
}
```

(import `"slices"`, `"meknik/synth"`).

`cmd/meknik/main_test.go`: delete `TestEnqueuePublishesAcceptedCue` and `TestEnqueuePublishesNothingForDroppedCue`; change `enqueue(make(chan mixer.Sound), newRNG(), func(string) {})` to `enqueue(make(chan mixer.Sound), newRNG())`; append:

```go
func TestAnnouncePublishesStartedCue(t *testing.T) {
	var published []string
	announce(func(line string) { published = append(published, line) })(mixer.Sound{Cue: synth.Done})
	if want := []string{"cue done"}; !slices.Equal(published, want) {
		t.Errorf("published = %q, want %q", published, want)
	}
}
```

- [ ] **Step 2: Run** `make test`; Expected: FAIL (too many arguments to `daemon.Play`; undefined `announce`).

- [ ] **Step 3: Implement**

`daemon/daemon.go`:

```go
// Play schedules queued sounds on the grid and mixes them into out block by block, applying queued gains, until idle passes with no new sound, or forever with NeverIdle.
func Play(m *mixer.Mixer, s *grid.Scheduler, sounds <-chan mixer.Sound, gains <-chan float64, out io.Writer, idle time.Duration, now func() time.Time, started func(mixer.Sound)) error {
	block := make([]float32, BlockFrames)
	var pcm []byte
	last := now()
	for idle == NeverIdle || now().Sub(last) < idle {
		last = drain(m, s, sounds, gains, last, now)
		for _, st := range s.Due(BlockFrames) {
			m.AddAt(st.Sound, st.Offset)
			started(st.Sound)
		}
		m.Fill(block)
		pcm = playback.Encode(pcm[:0], block)
		if _, err := out.Write(pcm); err != nil {
			return err
		}
	}
	return nil
}
```

`drain` takes the scheduler: `case snd := <-sounds:` does `if !s.Offer(snd) { slog.Warn("grid queue full, sound dropped", "cue", snd.Cue) }` then `last = now()`. Import `log/slog` and `"meknik/grid"`.

`cmd/meknik/main.go`:
- `enqueue` loses its `publish` parameter and the `publish(...)` call (the accepted case only sends).
- Add:

```go
func announce(publish func(string)) func(mixer.Sound) {
	return func(s mixer.Sound) { publish(events.CueLine(s.Cue)) }
}
```

- `transport.Serve(conn, enqueue(sounds, newRNG()))`.
- `daemon.Play(mixer.New(...), grid.New(), sounds, gains, stream, idle, time.Now, announce(hub.Publish))`; import `"meknik/grid"`.

- [ ] **Step 4:** `make test` all ok; `make lint` `0 issues.`
- [ ] **Step 5: Commit**

```bash
fossil changes
fossil commit -m "Play sounds on the rhythmic grid and announce cues when they start" daemon/daemon.go daemon/daemon_test.go cmd/meknik/main.go cmd/meknik/main_test.go
```

- [ ] **Step 6: Check by hand (controller, with the user)** — `make install && pkill -x meknik`; fire `return` and `done` together (`for c in return done; do printf $c | socat - UNIX-SENDTO:$HOME/.local/state/meknik/run/meknik.sock; done`): they play 125 ms apart; a burst of 10 ticks plays as a rhythmic run with drops; lamps flash in time.
