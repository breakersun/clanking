# meknik pan Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Each project's cues sound from a fixed stereo position derived from a hash of the hook payload's `cwd`.

**Architecture:** A new pure package `pan` maps a cwd to a pan value and a pan value to constant-power gains. The mixer becomes stereo (interleaved), with a `Pan` per sound; pw-cat plays two channels. The hook client carries `cwd` in the cue datagram (`<cue> <cwd>`), and the daemon sets each sound's pan from it.

**Tech Stack:** Go 1.26 in the podman build image, Fossil.

**Spec:** `docs/superpowers/specs/2026-10-08-meknik-pan-design.md`

## Global Constraints

- `pan.Width = 0.7`, `pan.Channels = 2`. `pan.Of("") == 0`.
- Hash: 64-bit FNV-1a (`hash/fnv`), mapped linearly onto `[-Width, +Width]`; negative is left.
- Gains: θ = (p+1)·π/4, left = cos θ, right = sin θ. So p = −1 gives left 1, right 0 exactly.
- Stereo blocks are interleaved `L R L R …`; a block of `n` frames is `Channels*n` floats. Frame offsets (`AddAt`, grid `Start.Offset`) stay in frames.
- Cue datagram: `string(cue) + " " + cwd`, split with `strings.Cut` on the first space. `maxDatagram = 8192`.
- No level compensation for the −3 dB centre loss.
- Test: `make test`. Lint: `make lint`. Both pass before each commit.
- Fossil: one-line commits, no trailers, each path listed; `fossil add` new files first; never git; never `fossil revert`/`fossil rm`.
- Code style per `~/.claude/CLAUDE.md`: no comments beyond one-line doc comments; one assertion per test; tests < 5 s; named constants; no defensive checks; no fallbacks; DRY.

## Review Focus

1. **Long cwd**: today's 64-byte datagram buffer would truncate any real path into a garbage cwd. A 4000-character cwd must round-trip intact. Test in Task 3.
2. **cwd with spaces**: the split is on the *first* space only. Test in Task 3.
3. **Payload without `cwd`, and the self-test**: both send an empty cwd and must play at centre, not at some hashed position. Test `pan.Of("")` in Task 1.
4. **`meknik play` speed**: once pw-cat takes two channels, mono samples would play at double speed and lose half their length. `Interleave` must double the length. Test in Task 1.
5. **Old daemon, new hook client**: a daemon from before Task 3 reads `tick /home/…` as an unknown cue and stays silent. Accepted: restart the daemon after installing. Done in Task 3's manual check.

---

### Task 1: `pan` package

**Files:** Create `pan/pan.go`, `pan/pan_test.go`.

**Interfaces — Produces:**
- `pan.Width float64 = 0.7`, `pan.Channels int = 2`
- `func Of(cwd string) float64`
- `func Gains(p float64) (left, right float64)`
- `func Interleave(samples []float32, p float64) []float32`

- [ ] **Step 1: Write the failing tests** — `pan/pan_test.go`:

```go
package pan_test

import (
	"math"
	"testing"

	"meknik/internal/approx"
	"meknik/pan"
)

const (
	tolerance = 1e-9
	meknikDir = "/home/rc/meknik"
	otherDir  = "/home/rc/lil.tel"
	spaceDir  = "/home/rc/My Projects/x"
	hardLeft  = -1.0
)

var someDirs = []string{meknikDir, otherDir, spaceDir, "/", "/tmp", "/home/rc", "/var/local/windows", "/home/rc/ia"}

func TestOfIsStable(t *testing.T) {
	if a, b := pan.Of(meknikDir), pan.Of(meknikDir); a != b {
		t.Errorf("Of twice = %v then %v, want equal", a, b)
	}
}

func TestOfStaysWithinWidth(t *testing.T) {
	for _, dir := range someDirs {
		t.Run(dir, func(t *testing.T) {
			if got := pan.Of(dir); math.Abs(got) > pan.Width {
				t.Errorf("Of(%q) = %v, want within ±%v", dir, got, pan.Width)
			}
		})
	}
}

func TestOfSeparatesDirectories(t *testing.T) {
	if a, b := pan.Of(meknikDir), pan.Of(otherDir); a == b {
		t.Errorf("Of(%q) = Of(%q) = %v, want different", meknikDir, otherDir, a)
	}
}

func TestOfCentresEmptyDirectory(t *testing.T) {
	if got := pan.Of(""); got != 0 {
		t.Errorf("Of(\"\") = %v, want 0", got)
	}
}

func TestCentreGainsAreEqual(t *testing.T) {
	left, right := pan.Gains(0)
	if !approx.Equal(left, right, tolerance) {
		t.Errorf("Gains(0) = %v, %v, want equal", left, right)
	}
}

func TestRightPanFavoursRight(t *testing.T) {
	if left, right := pan.Gains(pan.Width); right <= left {
		t.Errorf("Gains(%v) = %v, %v, want right louder", pan.Width, left, right)
	}
}

func TestGainsKeepPowerConstant(t *testing.T) {
	left, right := pan.Gains(pan.Width)
	if got := left*left + right*right; !approx.Equal(1, got, tolerance) {
		t.Errorf("left² + right² = %v, want 1", got)
	}
}

func TestInterleaveDoublesLength(t *testing.T) {
	if got := len(pan.Interleave(make([]float32, 3), 0)); got != pan.Channels*3 {
		t.Errorf("len = %d, want %d", got, pan.Channels*3)
	}
}

func TestInterleavePutsLeftFirst(t *testing.T) {
	if got := pan.Interleave([]float32{1}, hardLeft); got[0] != 1 {
		t.Errorf("first sample = %v, want 1", got[0])
	}
}

func TestInterleaveSilencesRightWhenHardLeft(t *testing.T) {
	if got := pan.Interleave([]float32{1}, hardLeft); got[1] != 0 {
		t.Errorf("second sample = %v, want 0", got[1])
	}
}
```

- [ ] **Step 2: Run** `make test`; Expected: FAIL (package `meknik/pan` does not exist).

- [ ] **Step 3: Implement** — `pan/pan.go`:

```go
// Package pan places each project's sounds in the stereo field.
package pan

import (
	"hash/fnv"
	"math"
)

// Width is the furthest a project sits from centre, where 1 is one speaker only.
const Width = 0.7

// Channels is the number of interleaved output channels.
const Channels = 2

const quarterTurn = math.Pi / 4

// Of returns the pan of a working directory, in [-Width, +Width] with negative left; an empty one is centre.
func Of(cwd string) float64 {
	if cwd == "" {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(cwd))
	return Width * (2*float64(h.Sum64())/math.MaxUint64 - 1)
}

// Gains returns the constant-power left and right gains for pan p in [-1, +1].
func Gains(p float64) (left, right float64) {
	theta := (p + 1) * quarterTurn
	return math.Cos(theta), math.Sin(theta)
}

// Interleave returns mono samples as left/right frames at pan p.
func Interleave(samples []float32, p float64) []float32 {
	left, right := Gains(p)
	out := make([]float32, 0, Channels*len(samples))
	for _, x := range samples {
		out = append(out, float32(left)*x, float32(right)*x)
	}
	return out
}
```

- [ ] **Step 4:** `make test` all ok; `make lint` `0 issues.`
- [ ] **Step 5: Commit**

```bash
fossil changes
fossil add pan/pan.go pan/pan_test.go
fossil commit -m "Add pan: cwd hash to stereo position and constant-power gains" pan/pan.go pan/pan_test.go
```

---

### Task 2: Stereo mixer and output

All of these land together: a stereo mixer feeding a one-channel pw-cat, or the reverse, plays at the wrong speed.

**Files:** Modify `mixer/mixer.go`, `mixer/mixer_test.go`, `daemon/daemon.go`, `daemon/daemon_test.go`, `playback/playback.go`, `playback/playback_test.go`, `cmd/meknik/main.go`.

**Interfaces:**
- Consumes: `pan.Gains`, `pan.Channels`, `pan.Interleave` (Task 1).
- Produces: `mixer.Sound.Pan float64`; `(*Mixer).Fill(block []float32)` now takes `pan.Channels*frames` interleaved floats.

- [ ] **Step 1: Write the failing tests**

`mixer/mixer_test.go`: existing tests read `block[0]` (the left sample) and expect it to equal the mono sum. Keep them exact by making the test sounds hard left (left gain exactly 1):

- Add the import `"meknik/pan"` and the constants `hardLeft = -1.0`, `hardRight = 1.0`.
- `constant` returns `mixer.Sound{Samples: samples, Cue: cue, Pan: hardLeft}`.
- Add `func newBlock() []float32 { return make([]float32, pan.Channels*blockSize) }`. Every `make([]float32, blockSize)` in the file becomes `newBlock()` (in `firstFrame`, `TestFinishedVoiceIsRetired`, `TestUnfinishedVoiceContinuesInNextBlock`, `TestAddAtDelaysFirstSample`, `TestAddAtLeavesEarlierFramesSilent`, `TestDelayedVoiceContinuesInNextBlock`).
- `TestAddAtDelaysFirstSample`: frame 2's left sample is `block[pan.Channels*2]`; check and report that index.
- `TestAddAtLeavesEarlierFramesSilent`: frame 1's left sample is `block[pan.Channels*1]`; check and report that index.

Add:

```go
func TestRightPannedSoundIsLouderOnRight(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.Add(mixer.Sound{Samples: []float32{loud}, Pan: pan.Width})
	block := newBlock()
	m.Fill(block)
	if block[1] <= block[0] {
		t.Errorf("left, right = %v, %v, want right louder", block[0], block[1])
	}
}

func TestHardRightSilencesLeft(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.Add(mixer.Sound{Samples: []float32{loud}, Pan: hardRight})
	block := newBlock()
	m.Fill(block)
	if math.Abs(float64(block[0])) > tolerance {
		t.Errorf("left = %v, want 0", block[0])
	}
}

func TestSecondFrameFollowsFirstPair(t *testing.T) {
	m := mixer.New(unity, wideCeiling)
	m.Add(mixer.Sound{Samples: []float32{quiet, loud}, Pan: hardLeft})
	block := newBlock()
	m.Fill(block)
	if !approx.Equal(loud, float64(block[pan.Channels]), tolerance) {
		t.Errorf("second frame left = %v, want %v", block[pan.Channels], loud)
	}
}
```

`daemon/daemon_test.go`:
- Add the import `"meknik/pan"` and the constant `hardLeft = -1.0`.
- `TestPlayWritesWholeBlocks`: the remainder modulus becomes `pan.Channels * daemon.BlockFrames * bytesPerSample`.
- `TestPlayWritesQueuedSound` and `TestPlayAppliesQueuedGain`: the queued sound becomes `mixer.Sound{Samples: []float32{sample}, Pan: hardLeft}`.

`playback/playback_test.go` (add `"slices"` to the imports):

```go
func TestCommandPlaysStereo(t *testing.T) {
	args := playback.Command().Args
	i := slices.Index(args, "--channels")
	if got := args[i+1]; got != strconv.Itoa(pan.Channels) {
		t.Errorf("--channels %s, want %d", got, pan.Channels)
	}
}
```

(also imports `"strconv"` and `"meknik/pan"`).

- [ ] **Step 2: Run** `make test`; Expected: FAIL (`mixer.Sound` has no field `Pan`; the stereo channel count is wrong).

- [ ] **Step 3: Implement**

`mixer/mixer.go`: add `"meknik/pan"` to the imports. `Sound` gains

```go
	Pan     float64
```

and its doc comment becomes `// Sound is a rendered cue queued for playback at a stereo pan.` `Fill`:

```go
// Fill renders the next block of interleaved stereo frames and retires finished voices.
func (m *Mixer) Fill(block []float32) {
	clear(block)
	frames := len(block) / pan.Channels
	for _, v := range m.voices {
		left, right := pan.Gains(v.Pan)
		for i := range frames {
			if j := v.pos + i; j >= 0 && j < len(v.Samples) {
				block[pan.Channels*i] += float32(left) * v.Samples[j]
				block[pan.Channels*i+1] += float32(right) * v.Samples[j]
			}
		}
		v.pos += frames
	}
	m.voices = slices.DeleteFunc(m.voices, func(v *voice) bool { return v.pos >= len(v.Samples) })
	for i, x := range block {
		block[i] = float32(m.gain * m.ceiling * math.Tanh(float64(x)/m.ceiling))
	}
}
```

`daemon/daemon.go`: add `"meknik/pan"` to the imports; in `Play`, `block := make([]float32, pan.Channels*BlockFrames)`. `s.Due(BlockFrames)` is unchanged (frames).

`playback/playback.go`: add `"meknik/pan"` to the imports; `"--channels", strconv.Itoa(pan.Channels),`; the `Command` doc comment says "interleaved stereo float32 PCM".

`cmd/meknik/main.go` `runPlay`: add `"meknik/pan"` to the imports; `stream.Write(playback.Encode(nil, pan.Interleave(samples, 0)))`.

- [ ] **Step 4:** `make test` all ok; `make lint` `0 issues.`
- [ ] **Step 5: Commit**

```bash
fossil changes
fossil commit -m "Mix and play in stereo with a pan per sound" mixer/mixer.go mixer/mixer_test.go daemon/daemon.go daemon/daemon_test.go playback/playback.go playback/playback_test.go cmd/meknik/main.go
```

- [ ] **Step 6: Check by hand (controller)**: `make install && pkill -x meknik`; `meknik play done` sounds normal (right pitch and length, centred); `meknik selftest` sounds as before, centred.

---

### Task 3: Carry `cwd` from hook to pan

**Files:** Modify `hookevent/hookevent.go`, `hookevent/hookevent_test.go`, `transport/transport.go`, `transport/transport_test.go`, `selftest/selftest.go`, `cmd/meknik/main.go`, `cmd/meknik/main_test.go`.

**Interfaces:**
- Consumes: `pan.Of` (Task 1), `mixer.Sound.Pan` (Task 2).
- Produces:
  - `hookevent.Event{Cue synth.Cue; Cwd string}`
  - `hookevent.Parse(r io.Reader) (Event, bool, error)`
  - `transport.Send(path string, c synth.Cue, cwd string) error`
  - `transport.Serve(conn *net.UnixConn, play func(synth.Cue, string)) error`
  - `enqueue(...) func(synth.Cue, string)`

- [ ] **Step 1: Write the failing tests**

`hookevent/hookevent_test.go`: in `TestParseMapsEvents`, the call becomes `ev, ok, _ := hookevent.Parse(...)` and the comparison becomes `result{ev.Cue, ok}`. Add:

```go
func TestParseReadsCwd(t *testing.T) {
	ev, _, _ := hookevent.Parse(strings.NewReader(`{"hook_event_name":"Stop","cwd":"/home/rc/meknik"}`))
	if ev.Cwd != "/home/rc/meknik" {
		t.Errorf("Cwd = %q, want %q", ev.Cwd, "/home/rc/meknik")
	}
}
```

`transport/transport_test.go`:
- Add the type `type delivery struct{ cue synth.Cue; cwd string }`.
- `serve` returns `<-chan delivery`; its play func is `func(c synth.Cue, cwd string) { got <- delivery{c, cwd} }`.
- `receive` takes and returns `delivery` (zero `delivery{}` on timeout).
- `sendCue(path, c)` calls `transport.Send(path, c, "")`.
- The existing receiving tests compare `receive(cues).cue`.
- Every other `transport.Send(path, X)` becomes `transport.Send(path, X, "")`.
- `TestServeReturnsNilWhenClosed`: `func(synth.Cue, string) {}`.
- In `TestDeliverSpawnsDaemonAndSends`, `var cues <-chan delivery`.

Add (with `"strings"` imported):

```go
const spacedCwd = "/home/rc/My Projects/meknik"

func TestSendCarriesCwdWithSpaces(t *testing.T) {
	path := socketPath(t)
	got := serve(t, path)
	_ = transport.Send(path, synth.Done, spacedCwd)
	if d := receive(got); d.cwd != spacedCwd {
		t.Errorf("cwd = %q, want %q", d.cwd, spacedCwd)
	}
}

func TestSendCarriesCueBesideCwd(t *testing.T) {
	path := socketPath(t)
	got := serve(t, path)
	_ = transport.Send(path, synth.Done, spacedCwd)
	if d := receive(got); d.cue != synth.Done {
		t.Errorf("cue = %q, want %q", d.cue, synth.Done)
	}
}

const longPathLength = 4000

func TestSendCarriesLongCwd(t *testing.T) {
	path := socketPath(t)
	got := serve(t, path)
	long := "/" + strings.Repeat("d", longPathLength)
	_ = transport.Send(path, synth.Tick, long)
	if d := receive(got); d.cwd != long {
		t.Errorf("cwd length = %d, want %d", len(d.cwd), len(long))
	}
}
```

`cmd/meknik/main_test.go` `TestEnqueueDropsWhenQueueIsFull`: the call becomes `enqueue(make(chan mixer.Sound), newRNG())(synth.Done, "")`. Add (with `"meknik/pan"` imported):

```go
func TestEnqueuePansByCwd(t *testing.T) {
	sounds := make(chan mixer.Sound, 1)
	enqueue(sounds, newRNG())(synth.Done, "/home/rc/meknik")
	if got := (<-sounds).Pan; got != pan.Of("/home/rc/meknik") {
		t.Errorf("Pan = %v, want %v", got, pan.Of("/home/rc/meknik"))
	}
}
```

- [ ] **Step 2: Run** `make test`; Expected: FAIL (`ev.Cue` undefined; too many arguments to `transport.Send`).

- [ ] **Step 3: Implement**

`hookevent/hookevent.go`:

```go
// Event is a hook's cue and the working directory of the session that raised it.
type Event struct {
	Cue synth.Cue
	Cwd string
}
```

`payload` gains `Cwd string `json:"cwd"``. `Parse` is now documented as `// Parse reads a hook payload and returns its event, reporting false when the event has no cue.` and has this body:

```go
func Parse(r io.Reader) (Event, bool, error) {
	var p payload
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return Event{}, false, err
	}
	c, ok := cue(p)
	return Event{Cue: c, Cwd: p.Cwd}, ok, nil
}

func cue(p payload) (synth.Cue, bool) {
	if p.Event == preToolUse && p.Tool == askToolName {
		return synth.Ask, true
	}
	if p.Event == messageDisplay && p.Index == firstDelta {
		return synth.Reply, true
	}
	c, ok := eventCues[p.Event]
	return c, ok
}
```

`transport/transport.go`:
- Add `"strings"` to the imports.
- The package doc says "carries cue names and working directories".
- Set `maxDatagram = 8192` and add `fieldSeparator = " "` to the const block.
- The `Serve` doc comment becomes `// Serve hands each received cue and working directory to play until the connection is closed.` Its signature and receive line become:

```go
func Serve(conn *net.UnixConn, play func(synth.Cue, string)) error {
	...
		cue, cwd, _ := strings.Cut(string(buf[:n]), fieldSeparator)
		play(synth.Cue(cue), cwd)
```

- The `Send` doc comment becomes `// Send delivers one cue and the sender's working directory to the daemon socket.`, with the signature `func Send(path string, c synth.Cue, cwd string) error` and the write line `_, err = conn.Write([]byte(string(c) + fieldSeparator + cwd))`.

`selftest/selftest.go` `RunDaemon`: `transport.Send(cuePath, c, "")`.

`cmd/meknik/main.go`:
- `runHook`:

```go
	ev, ok, err := hookevent.Parse(os.Stdin)
	...
	return transport.Deliver(func() error { return transport.Send(socket, ev.Cue, ev.Cwd) }, spawn, transport.SpawnTimeout, transport.RetryInterval)
```

- `enqueue` returns `func(c synth.Cue, cwd string)` and sends `mixer.Sound{Samples: samples, Cue: c, Pan: pan.Of(cwd)}`.

- [ ] **Step 4:** `make test` all ok; `make lint` `0 issues.`
- [ ] **Step 5: Commit**

```bash
fossil changes
fossil commit -m "Pan each hook cue by its session's working directory" hookevent/hookevent.go hookevent/hookevent_test.go transport/transport.go transport/transport_test.go selftest/selftest.go cmd/meknik/main.go cmd/meknik/main_test.go
```

- [ ] **Step 6: Check by hand (controller)**: `make install && pkill -x meknik` (the old daemon cannot read the new datagrams). Then fire hooks from two directories:

```bash
for d in /home/rc/meknik /home/rc/lil.tel; do echo "{\"hook_event_name\":\"Stop\",\"cwd\":\"$d\"}" | meknik hook; sleep 1; done
```

The two `done` cues come from audibly different positions. Then ask the user for an ear check in live sessions.
