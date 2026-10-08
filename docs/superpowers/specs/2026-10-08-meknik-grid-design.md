# meknik rhythmic grid

## Purpose

Cues that arrive together, such as `return` and `done`, play on top of
each other. Quantize every sound to a rhythmic grid so that at most one
sound starts at a time, keep a short queue, and drop sounds in a burst
rather than pile them up.

Non-goals: grids finer than 32nd notes; any other burst handling (to be
handled separately).

## Rules

- **Tempo**: 120 bpm. A beat is `synth.SampleRate * 60 / 120` = 24 000
  frames. A 16th note is 6000 frames (125 ms); a 32nd note is 3000
  frames (62.5 ms).
- **Grid**: grid lines are multiples of the slot length on the daemon's
  frame clock, which starts at 0. Every 16th line is also a 32nd line,
  so a change of slot length never shifts the phase.
- **One per line**: a sound starts on the first grid line at or after
  both the current frame and the line after the previous start. At most
  one sound starts per line.
- **Queue**: every sound, ticks included, waits in a FIFO queue of at
  most 4. A sound that arrives while 4 are waiting is dropped. The
  newest sound is the one dropped.
- **Faster**: a sound that is accepted ends the drop streak. When a
  third drop in a row occurs, the slot length becomes a 32nd note. It
  never becomes shorter.
- **Back to 16ths**: while at 32nds, after a quiet stretch of one bar
  (4 beats, 96 000 frames, 2 s) with no sound arriving, the slot length
  returns to a 16th note.

## Components

- `grid` (new, GTK-free): `Scheduler` with
  `Offer(mixer.Sound) bool` (accept or drop) and
  `Due(frames int) []Start`, which returns the sounds that start in the
  next `frames` frames with their frame offsets, and advances the clock.
  `Start{Sound mixer.Sound; Offset int}`.
- `mixer`: `Sound.Cue synth.Cue` replaces `Sound.Tick`; the voice-pool
  rule asks `Cue == synth.Tick`. `AddAt(sound, offset)` starts a sound
  `offset` frames into the next block; `Add` is `AddAt(sound, 0)`.
- `daemon.Play`: arriving sounds go to `Scheduler.Offer`; a drop is
  logged. Before each block it starts every due sound with `AddAt` and
  reports it to a `started func(mixer.Sound)` callback.
- `cmd/meknik`: `enqueue` no longer publishes events; the daemon's
  `started` callback publishes `events.CueLine` when a sound actually
  starts. Lamps therefore flash in time with what plays, and dropped
  sounds never flash.

## Effects

The longest added delay is one slot plus a full queue: 5 × 125 ms =
0.625 s at 16ths. The self-test spaces its cues far apart, so it is
unaffected.

## Testing

`grid`, one assertion each: the first sound starts at offset 0 on frame
0; a second sound starts one 16th later; a sound arriving between lines
waits for the next line; a fifth waiting sound is dropped; a drop then
an acceptance resets the streak; the third drop in a row switches to
32nds; one quiet bar returns to 16ths; a sound due after the block is
not returned. `mixer`: `AddAt` delays the first sample by the offset; a
delayed voice continues into the next block. `daemon`: `Play` reports a
started sound.
