# meknik self-test

## Purpose

Like a dashboard that lights every lamp when the key turns, a self-test
plays every meknik cue once, in sequence, so the user hears that the
whole path works: cue socket, daemon, mixer, and `pw-cat`. It is
available as `meknik selftest` and as a button in `meknik-ui`.

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

The cue order is the order of the `synth` cue constants: tick, spawn,
return, done, failed, permission, ask, reply, milestone, noted.
`synth.Cues` is a new exported slice in that order.

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

A `Self-test` button below the Mute button. It runs `selftest.Run` on
its own goroutine, not through the request worker, so that mute and
volume changes still go through while a test plays. The button is
insensitive while a test runs and becomes sensitive again when it ends.
A refusal or other error is logged and shown in the error label.

## Testing

Unit tests, one assertion each, in `selftest`:

- The cues are sent in `synth.Cues` order.
- Each wait is the cue's length plus `cuePause`.
- While muted, `Run` returns `ErrMuted`.
- While muted, no cue is sent.
- A status error is returned as is.

`synth`: `Cues` holds every cue that `Sound` knows, once.

The UI button is checked by hand: press it, hear every cue, and see the
button disabled until the sequence ends; mute, press it, and see the
refusal in the error label.
