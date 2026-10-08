# meknik lamps and events

## Purpose

Give `meknik-ui` a row of lamps, one per cue, that flash when the daemon
plays a cue and fade back to dim over about 400 ms. Keep every open window
in step with the daemon's volume and mute, whoever changes them. Also
give the window an even inner margin.

The daemon gains an events socket that streams what it does. The
window watches it.

Non-goals: lamp brightness scaled by volume, labels under lamps, any
CLI command that reads events.

## Events socket

The daemon listens on a third Unix stream socket,
`$XDG_STATE_HOME/meknik/run/meknik.events` (`statedir.EventsSocket`).
Each connection receives a stream of newline-terminated event lines
until it disconnects or the daemon stops. Clients send nothing.

Two kinds of line:

- `cue <name>`, such as `cue spawn`: the daemon accepted the cue into
  the mixer. A cue dropped because the sound queue is full, or an
  unknown cue, produces no event. Cue events are sent whether or not
  meknik is muted, so a muted meknik is a silent visual monitor.
- `settings <status line>`, such as `settings volume -6 dB, muted`:
  the settings changed. The status line is `control.Settings.String`,
  read back with `control.ParseSettings`.

On connect, the first line is a `settings` line with the current
settings, so a new or reconnecting watcher starts in step. It comes
from `controller.Handle` with a `status` request.

## Fan-out

A new GTK-free package, `events`, holds:

- `Hub`, which fans each published line out to every subscriber.
  `Subscribe` returns a receive channel with a buffer of
  `subscriberQueue` lines and an unsubscribe function. `Publish` never
  blocks: when a subscriber's buffer is full, that line is dropped for
  that subscriber only. Audio never waits on a slow window.
- `CueLine(synth.Cue) string` and `SettingsLine(control.Settings) string`,
  which format the two kinds of line.
- `Dispatch(line string, cue func(synth.Cue), settings func(control.Settings)) error`,
  which parses one line and calls the matching function. An unknown
  kind or an unreadable settings line returns `events.ErrLine`.

In the daemon, `enqueue` publishes `CueLine` after a cue is accepted,
and the controller's apply function publishes `SettingsLine` after
each change.

## Transport

- `transport.ListenControl` is renamed `transport.ListenStream`, since
  it now binds both stream sockets.
- `transport.ServeEvents(l net.Listener, subscribe func() (<-chan string, func()), snapshot func() string) error`
  accepts connections until the listener closes. For each connection
  it subscribes, writes the snapshot line, then copies lines from the
  subscription to the connection until a write fails, then
  unsubscribes and closes.
- `transport.Watch(path string, line func(string)) error` connects to
  the events socket and calls `line` for each line, without the
  newline, until the stream ends. It returns the read error, or nil at
  a clean end.

## Window

- **Margin**: the main box gets a `windowMargin` (12 px) margin on all
  four sides. Row spacing stays at 6 px.
- **Lamps**: a horizontal row of 10 small round lamps, in `synth.Cues`
  order, above the volume scale. Each lamp has its own colour from the
  icon's LCARS palette and a tooltip with the cue name. The lamps look
  like LEDs: a 2 px dark bezel, an inner shadow, and a fixed glossy
  highlight (a radial gradient) on the lens. An unlit lamp shows its
  own colour dimmed (`shade(colour, 0.32)`), like a switched-off LED.
  The base style has
  `transition: background-color 400ms ease-out, box-shadow 400ms ease-out`.
  A cue adds the lamp's `lit` class, which sets the full colour and a
  glow in that colour (`box-shadow 0 0 8px 2px`) with no transition.
  The class is removed after a 40 ms hold (`lampHold`), and then the
  CSS transition fades the colour and the glow back to dim. A cue
  arriving during a fade lights it again.
  The style sheet is one CSS string in the binary, loaded with a
  `gtk.CSSProvider` for the display.
- **Watching**: a goroutine runs `transport.Watch` in a loop. Each line
  goes to the main loop through `glib.IdleAdd` and then through
  `events.Dispatch`. A cue lights its lamp. A settings event goes
  through the existing `show`, so the volume scale and mute button
  follow every change, from any client, without sending a request.
  When the stream ends, the error is logged and the goroutine waits
  `reconnectInterval` (1 s) before it connects again.

## Testing

Unit tests, one assertion each:

- `events`: a subscriber receives published lines in order; `Publish`
  returns while a subscriber's buffer is full; an unsubscribed channel
  receives nothing more; `CueLine` and `SettingsLine` produce the
  documented lines; `Dispatch` calls the cue function for a cue line
  and the settings function for a settings line; `Dispatch` returns
  `ErrLine` for an unknown kind and for an unreadable settings line.
- `transport`: a watcher first receives the snapshot line; it then
  receives a published line; `Watch` returns when the server closes the
  connection.
- `statedir`: `EventsSocket` lives in the run directory.

Checked by hand: the lamps flash and fade; `meknik selftest` sweeps
them in order; `meknik mute` and `meknik volume -12` from a terminal
move the window's controls; a second window follows the first.
