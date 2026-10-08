# meknik tempo

## Purpose

`meknik tempo <bpm>` sets the tempo of the rhythmic grid. The tempo is a
saved setting, like volume and mute.

Non-goals: tempo controls in `meknik-ui`.

## Command

`meknik tempo <bpm>` takes a whole number from `control.MinTempoBPM`
(60) to `control.MaxTempoBPM` (240). Anything else is a usage error,
reported before any daemon is contacted, as for `volume`.

## Setting

- `control.Settings` gains `TempoBPM int` (`json:"tempo_bpm"`).
- `control.Load` starts from `Settings{TempoBPM: DefaultTempoBPM}` (120)
  and reads the file over it, so a missing file or a file without the
  field yields 120.
- `Settings.String()` is three parts joined by `, `:
  `volume 0 dB, unmuted, 120 bpm`. `ParseSettings` splits on `, `,
  requires exactly three parts, and reads each with the same formats
  that `String` writes. A line with extra parts is rejected.
- Event `settings` lines carry the tempo too. A `meknik-ui` window
  started before this change fails to read them until it is restarted.

## Status line

- The grid part no longer names the tempo: `in 16ths, 0 queued,
  0 dropped`. The tempo comes from the settings part, which the daemon
  computes when it replies, so `meknik tempo 90` prints `90 bpm` at
  once.
- The CLI joins the two parts with a space, so the printed line is
  unchanged in shape:
  `volume 0 dB, unmuted, 120 bpm in 16ths, 0 queued, 0 dropped`.

## Grid

- `grid.New(bpm int)` and `(*Scheduler).SetTempo(bpm int)`.
- The scheduler keeps its division (16 or 32) and the length of a
  whole note in frames, `4 * synth.SampleRate * 60 / bpm`. A slot is
  the whole note divided by the division; the quiet stretch is one bar,
  which is one whole note.
- Grid lines stay multiples of the slot from frame 0, and the next
  start is rounded up from the end of the previous slot, so a tempo
  change never starts two sounds on one line.

## Daemon

`daemon.Play` takes a `<-chan control.Settings` in place of the gains
channel. Each change sets the mixer gain and the scheduler tempo. The
controller's apply function sends the whole settings value.

## Testing

One assertion each. `control`: `tempo 90` parses; `tempo 59`,
`tempo 241`, and `tempo fast` are rejected; tempo applies; the status
line round-trips with tempo; a line with an extra part is rejected;
`Load` without a file gives 120. `grid`: at 90 bpm a second sound
starts 8000 frames after the first; `Due` refreshes the snapshot's
queued count; `State.String` has no tempo. `daemon`: `Play` applies a
queued tempo. `cmd/meknik`: an out-of-range tempo is rejected before
any daemon is contacted; the status line joins with a space.
