# meknik self-test

## Purpose

Like a dashboard that lights every lamp when the key turns, a self-test
plays every meknik cue once, in sequence, so the user hears that the
whole path works: cue socket, daemon, mixer, and `pw-cat`. It is
available as `meknik selftest`.

Non-goals: a window title bar or an About window (the compositor's
border-only decoration is intended), changes to the daemon or the
control protocol.

## Behavior

1. Send `status` through `transport.Control`, which starts the daemon
   when none answers.
2. If the daemon is muted, stop with `selftest.ErrMuted`
   ("muted; unmute to run the self-test"). Nothing plays. Mute is
   always honored.
3. Otherwise send each cue in `synth.Cues` over the cue socket with
   `transport.Send`, the same path hooks use. After each cue, wait for
   the cue's own length plus `cuePause` (400 ms), so that each cue is
   heard alone. The length is the synthesized sample count divided by
   `synth.SampleRate`.

The cue order is tick, spawn, return, failed, permission, ask, reply,
milestone, noted, done. The sequence ends on `done`, the completion
chord, so the test closes on a "complete" sound. `synth.Cues` is a new
exported slice in that order.

Muting during a test silences the remaining cues at once, because the
daemon's mixer applies mute to every cue. The test does not check mute
again.

## Package

`selftest` is a new GTK-free package:

```go
// Run plays every cue once through the daemon, refusing while muted.
func Run(status func() (control.Settings, error), send func(synth.Cue) error, sleep func(time.Duration)) error
```

The callers supply `status`, `send`, and `sleep`, so that tests can
record the cues and the waits without a daemon or real time.

The cue socket name moves from `cmd/meknik` to `statedir.CueSocket`
(`meknik.sock`), because both binaries now need it.

## CLI

`meknik selftest` runs `selftest.Run` and blocks until the last cue has
played. While muted it prints the error and exits 1. The usage line
gains `selftest`.

## UI

`meknik-ui` has no self-test button; it was removed on 2026-10-08 to
keep the window small. The self-test is a command-line feature only.
While `meknik selftest` runs, the window's lamps light in sequence.

## Testing

Unit tests, one assertion each, in `selftest`:

- The cues are sent in `synth.Cues` order.
- Each wait is the cue's length plus `cuePause`.
- While muted, `Run` returns `ErrMuted`.
- While muted, no cue is sent.
- A status error is returned as is.

`synth`: `Cues` holds every cue that `Sound` knows, once.
