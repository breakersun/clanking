# meknik grid status

## Purpose

`meknik status` (and every other control command, which reply with
status) also reports the rhythmic grid: tempo, current division, how
many sounds wait in the queue, and how many have been dropped since the
daemon started.

```
$ meknik status
volume 0 dB, unmuted, 120 bpm in 32nds, 3 queued, 41 dropped
```

Non-goals: grid state in `meknik-ui` or on the events stream.

## Grid snapshot

`grid.Scheduler` counts every dropped sound (`Dropped`, never reset).
After `New`, every `Offer`, and every `Due`, it stores a
`grid.State{Division, Queued, Dropped int}` in an `atomic.Pointer`.
`Division` is 16 or 32 (a whole note is 4 beats = 96 000 frames, divided
by the slot length). `(*Scheduler).State() State` loads the snapshot,
so the control goroutine reads it without a lock and the audio loop
never waits for a status request. `State.String()` is
`120 bpm in 16ths, 0 queued, 0 dropped`.

## Protocol

A control reply is `<settings>; <grid>`, for example
`volume 0 dB, unmuted; 120 bpm in 32nds, 3 queued, 41 dropped`. The
separator `; ` keeps the settings part, which itself contains a comma,
unambiguous for `control.ParseSettings`.

- `transport.ServeControl(l, handle, describe func() string)` appends
  `; ` and `describe()` to each successful reply. The daemon passes
  `sched.State().String`.
- `transport.Control` returns `(control.Settings, string, error)`: it
  cuts the reply at `; `, parses the settings part, and returns the
  grid part.

## Clients

- CLI: prints `<settings>, <grid>` for every control command.
- `meknik-ui` and `selftest` ignore the grid part.
- Event `settings` lines are unchanged.

## Testing

One assertion each. `grid`: a new scheduler reports 16ths, nothing
queued or dropped; queued sounds are counted; drops are counted; the
third drop in a row reports 32nds; `State.String` formats as above.
`transport`: `Ask` returns `<settings>; <grid>`; `Control` returns the
grid part; `Control` still parses the settings part. `cmd/meknik`: the
printed status line joins settings and grid with `, `.
