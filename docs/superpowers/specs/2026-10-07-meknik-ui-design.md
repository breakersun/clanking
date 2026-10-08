# meknik-ui: a GTK 4 volume and mute window

## Purpose

Give a small desktop window that sets the meknik daemon's master volume
and mutes or unmutes it, as an alternative to `meknik volume`,
`meknik mute`, and `meknik unmute`. Success: on open, the controls show
the daemon's current settings; each change takes effect at once and
persists through the daemon's existing `mixer.json`.

Non-goals: a tray icon, a layer-shell overlay, live updates when another
client changes the settings while the window is open, libadwaita.

## Constraints

- Runs on the host (bam: Debian 13.7, Wayland, umbriel). Not in llmgaol.
- GTK 4 through gotk4. The host needs only the `libgtk-4-1` runtime.
- The daemon and the control protocol do not change.
- All builds, tests, and lint run in a podman build image, so the
  toolchain is stable and does not depend on the host.

## Window

`meknik-ui` is a separate binary, `cmd/meknik-ui`. It creates a
`gtk.Application` with ID `tel.lil.meknik` and builds one window,
titled `meknik`, in its `activate` handler. The window holds:

- A horizontal `gtk.Scale` for volume in dB, with its value shown. The
  range is `minVolumeDB` (−60 dB) to `control.MaxVolumeDB` (+24 dB),
  in 0.5 dB steps.
- A `gtk.ToggleButton` for mute. Pressed means muted.
- A label that shows the last request error, empty when there is none.

## Data flow

The window is a client of the control socket,
`$XDG_STATE_HOME/meknik/run/meknik.ctl`, and sends the same request
lines as the CLI through `transport.Ask`.

- On open, the window sends `status` and sets both controls from the
  reply.
- The toggle button sends `mute` or `unmute`, the explicit state it
  shows, not `toggle`, so the button cannot invert the daemon's state.
- Scale changes are debounced by `volumeDebounce` (100 ms), with a
  `glib.TimeoutAdd` source that each change replaces, and then send
  `volume <dB>`, so a drag does not flood the socket.
- Each reply sets both controls again. Setting a control from a reply
  does not send a request.

Socket calls run off the GTK main thread. Results return to the main
thread through `glib.IdleAdd`.

The status reply is a human-readable line such as
`volume -6 dB, muted`. A new `control.ParseSettings` parses that line
back to `control.Settings`. A round-trip test against `Settings.String`
keeps the two in step.

## Starting the daemon

When no daemon answers, the window starts one and retries, through the
same `transport.Deliver` path as the CLI. It runs `meknik daemon` from
its own install directory, `~/.local/share/meknik/bin/`, where both
binaries are installed.

Shared code moves out of `cmd/meknik/main.go` so that both binaries
use one implementation. `stateDir`, `statePath`, `runPath`, and
`openLog` move to a new package, `statedir`. `spawnDaemon` and
`daemonCommand` move to the existing `daemon` package as `daemon.Spawn`,
which takes the path of the `meknik` binary as an argument: the CLI
gives `os.Executable()`, and the window gives `meknik` next to its own
executable.

## Errors

A failed request is logged through `slog` to the shared `meknik.log`
and shown in the error label. The window does not retry beyond what
`transport.Deliver` does.

## Build environment

A `Containerfile` at the repository root defines the build image,
`localhost/meknik-build`:

- Base `debian:13.7`, the same release as bam, so the binaries link
  against the same glibc and GTK 4.18.
- From trixie: `libgtk-4-dev`, `build-essential`, `pkg-config`.
- From trixie-backports: `golang-1.26-go` (1.26.8). `GOTOOLCHAIN=local`
  stops Go from downloading another toolchain.
- golangci-lint v2.12.2, built with
  `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`.
  It is not packaged in Debian.

`mise.toml` is deleted: the image is the only place that pins toolchain
versions. `go.mod` keeps `go 1.26.7`.

## Makefile

- `image` builds `localhost/meknik-build` with `podman build`.
- `test`, `lint`, and `build` run in the image through one shared
  `podman run` prefix. It mounts the repository at `/src:Z` and keeps
  the Go module and build caches in the named volumes `meknik-gomod`
  and `meknik-gocache`, so gotk4 compiles once. Rootless podman maps
  the container's root to the invoking user, so outputs belong to that
  user.
- `build` writes `bin/meknik` with `CGO_ENABLED=0`, which keeps it
  static for llmgaol's container-bin, and `bin/meknik-ui` with cgo.
- `install` runs on the host. It copies both binaries to
  `~/.local/share/meknik/bin`, links both into `~/.local/bin`, and
  copies only `meknik` to `~/.local/share/meknik/container-bin`.

`bin/` is added to Fossil's `ignore-glob`.

## Package layout

- `cmd/meknik-ui`: GTK wiring only. It is kept thin because it is not
  unit-tested.
- `statedir`: state and run paths, and the log file. New.
- `daemon`: gains `Spawn`.
- `control`: gains `ParseSettings`.

## Testing

- Unit tests, one assertion each: `ParseSettings` round trip and
  rejection of malformed lines, `statedir` paths, and the command that
  `daemon.Spawn` builds.
- The GTK wiring is checked by hand: run `bin/meknik-ui`, read the
  initial state, drag the scale, toggle mute, and confirm each change
  with `meknik status`.
- `make test` and `make lint` run in the build image and must pass
  before each commit.
