# meknik: synthesized audio feedback for Claude Code

## Purpose

Give subtle, synthesized audio cues for what Claude Code agents are
doing, the way an old hard drive or a Star Trek console gives ambient
feedback. Two goals for the first pass:

1. **Ambient awareness**: hear that agents are working, and roughly
   how busy they are, from the density of short ticks.
2. **Attention cues**: hear when an agent finishes, fails, or waits
   on the user, with each case audibly distinct.

Non-goals for the first pass: per-agent voices (pitch, timbre, or pan
per session), a continuous background bed, a plugin package, sample
playback of any kind.

## Constraints

- All sound is synthesized in code. No sample files.
- Subtle: quiet by default, and bursts from many agents must not
  dominate the soundscape.
- Many concurrent agents are the normal case.
- A hook must never block or alter Claude Code behavior.
- Linux with PipeWire. Output goes through `pw-cat`.

## Architecture

One Go binary, `meknik`, with three subcommands.

### `meknik hook` (client)

Configured as an **async** command hook for each mapped event, so it
never adds latency to Claude Code and its exit status cannot block a
tool call.

1. Read the hook JSON from stdin.
2. Map it to a cue name (see the mapping table). If there is no cue,
   exit.
3. Send the cue name as one datagram to the Unix datagram socket
   `$XDG_RUNTIME_DIR/meknik.sock`.
4. If the send fails, spawn `meknik daemon` detached (new session,
   stdio on `/dev/null`, working directory `/`), then retry the send
   until it succeeds or `SpawnTimeout` (1 s) elapses.

Errors go to the log and the process exits non-zero (exit code 1). It
writes nothing to stdout.

### `meknik daemon`

1. Take an exclusive `flock` on `$XDG_RUNTIME_DIR/meknik.lock` and hold
   it for the life of the process. If the lock is already held, exit
   0: another daemon is running. This makes concurrent autostarts
   safe without coordination in the client.
2. Remove any stale socket file, then bind the socket.
3. Start one `pw-cat` playback child that reads raw PCM from stdin:
   48 kHz, mono, float32, with `application.name=meknik` so it shows
   as its own stream in the mixer.
4. Run the audio loop: every block of `BlockFrames` (256) frames, mix
   the active voices into the block, apply the limiter, and write the
   block to the `pw-cat` pipe. When no voice is active, write silence,
   so the stream stays open and latency stays low. The pipe write
   paces the loop.
5. Exit when no event has arrived for `IdleTimeout` (10 min). The next
   hook starts a new daemon.

The `pw-cat` writer is passed to the audio loop as an `io.Writer`.

### `meknik play <cue>`

Render one cue and write it straight to a fresh `pw-cat`. This is for
tuning the sounds by ear, and needs no daemon.

## Event to cue mapping

| Hook event | Condition | Cue |
| --- | --- | --- |
| `PreToolUse` | `tool_name == "AskUserQuestion"` | `ask` |
| `PreToolUse` | any other tool | `tick` |
| `SubagentStart` | | `spawn` |
| `SubagentStop` | | `return` |
| `Stop` | | `done` |
| `StopFailure` | | `failed` |
| `PermissionRequest` | | `permission` |
| any other event | | none |

Only events in this table are added to `settings.json`. Unknown events
map to no cue, so a stray configuration plays nothing.

## Cue design

Pitches are equal-tempered, given in semitones above the tonic
`Tonic` (D5, 587.33 Hz). Every note has a short attack and a release
to zero, so no cue starts or ends with a click. Unless stated
otherwise, the waveform is a sine.

| Cue | Character | Definition |
| --- | --- | --- |
| `tick` | a disk seek; bursts patter | one sine burst, 12 ms, exponential decay, frequency random in 1800–2600 Hz, level random within ±3 dB |
| `spawn` | small bloop up | +12 then +19, 40 ms each |
| `return` | the mirror image | +19 then +12, 40 ms each |
| `done` | settled, resolved | +7, +4, 0, 90 ms apart, each ringing about 200 ms |
| `failed` | wrong, but not an alarm | −6 then −12, 150 ms each, with added 3rd and 5th harmonics for a buzzier timbre |
| `permission` | a knock, then tension | +5, +5 at 60 ms each, then +11 held 250 ms, unresolved |
| `ask` | a spoken "hm?" | one tone gliding from 0 to +7 over 250 ms |

Levels: `tick` peaks at −36 dBFS. The other cues peak at −30 dBFS.
`MEKNIK_GAIN` (dB, default 0) in the daemon's environment shifts all
levels. The mixer stream volume is the only mute control.

## Mixing and subtlety

- At most `MaxVoices` (8) voices sound at once.
- When the voice pool is full, a new `tick` is dropped. Any other new
  cue replaces the oldest `tick` voice. If the pool holds no `tick`,
  the new cue is dropped.
- A soft limiter (tanh curve) holds the mix below −24 dBFS, before `MEKNIK_GAIN` is applied.

## Package layout

| Package | Responsibility |
| --- | --- |
| `synth` | oscillators, envelopes, and the cue definitions, as pure functions that return sample buffers |
| `mixer` | voice pool, drop policy, limiter, and block rendering |
| `hookevent` | hook JSON to cue name |
| `transport` | socket client and server, autostart with an injected spawner |
| `cmd/meknik` | subcommands, configuration from the environment, wiring |

Logging uses `log/slog`, written to `$XDG_STATE_HOME/meknik/meknik.log`.

## Installation

- `go install ./cmd/meknik` puts `meknik` on the PATH.
- In `~/.claude/settings.json`, add one async command hook per mapped
  event, each running `meknik hook`. The client reads
  `hook_event_name`, so every event uses the same command.

## Testing

All tests run with `make test` (`go test ./...`), without an audio
device, and each test finishes in milliseconds. Lint with `make lint`
(`golangci-lint run`).

- `synth`: for each cue, check the duration, that the peak stays under
  its level, that the first and last samples are silent, and that each
  note segment has the expected pitch (zero-crossing estimate).
- `mixer`: sum of voices, limiter ceiling, and the drop policy when the
  pool is full.
- `hookevent`: a table of hook JSON to expected cue, including unknown
  events and `AskUserQuestion`.
- `transport`: a client and server round trip over a socket in a
  temporary directory, and autostart through an injected spawner.
- By ear: `meknik play <cue>`, the only manual check.
