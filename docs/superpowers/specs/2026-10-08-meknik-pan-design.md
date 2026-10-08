# meknik pan

## Purpose

Each project sounds from its own place in the stereo field, so the
listener can tell concurrent Claude Code sessions apart by ear. The
place comes from a hash of the hook payload's `cwd`, so it is the same
across `/clear` and restarts. Sessions in the same directory share a
place.

Non-goals: showing a project's pan in `meknik status` or `meknik-ui`;
configuring or overriding a project's pan.

## Pan

New package `pan`:

- `pan.Width = 0.7`; `pan.Channels = 2`.
- `pan.Of(cwd string) float64` hashes `cwd` with 64-bit FNV-1a and maps
  the hash linearly onto `[-Width, +Width]`. Negative is left. An empty
  cwd is centre, 0.
- `pan.Gains(p float64) (left, right float64)` is the constant-power
  law: θ = (p+1)·π/4, left = cos θ, right = sin θ.
- `pan.Interleave(samples []float32, p float64) []float32` returns the
  samples as interleaved left/right frames at pan `p`.

## Wire

- `hookevent.Parse` also reads `cwd` from the payload and returns it
  with the cue.
- The cue datagram is `<cue> <cwd>`. `transport.Send` takes the cwd;
  `transport.Serve` splits on the first space and calls
  `play(cue, cwd)`. Cue names have no spaces; paths may. The datagram
  buffer grows from 64 to 8192 bytes, room for a `PATH_MAX` path.
- `meknik selftest` sends an empty cwd, which the daemon plays at
  centre.
- `meknik play` writes to its own pw-cat, not the daemon; it writes
  `pan.Interleave(samples, 0)`.

## Mixer

- `mixer.Sound` gains `Pan float64`. The zero value is centre.
- The daemon's enqueue sets `Pan: pan.Of(cwd)`.
- `Fill` takes an interleaved stereo block of `2*frames` floats. Each
  voice adds `left*x` and `right*x` to its frame's two samples, then
  the tanh limiter and gain apply to every sample as before.
- `daemon.Play` allocates `2*BlockFrames` floats per block. Frame
  offsets from the grid stay in frames.

## Output

`playback` runs `pw-cat --channels 2`.

## Level

A centred sound is about −3 dB per channel compared with the mono
output it replaces; total power is unchanged. No compensation.

## Testing

One assertion each. `pan`: the same path gives the same pan; pans of a
set of paths stay within ±Width; two sample paths differ; centre gains
are equal; at +Width the right gain is larger. `hookevent`: the cwd is
extracted. `transport`: a cwd with spaces round-trips. `mixer`: a sound
panned right is louder on the right; the block is interleaved
left/right. `playback`: the pw-cat arguments ask for two channels.
