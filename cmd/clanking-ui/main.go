// Command clanking-ui is a GTK 4 window that sets the clanking daemon's volume and mute.
package main

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/chrchr/clanking/control"
	"github.com/chrchr/clanking/daemon"
	"github.com/chrchr/clanking/events"
	"github.com/chrchr/clanking/statedir"
	"github.com/chrchr/clanking/transport"
)

const (
	appID             = "io.github.chrchr.clanking"
	title             = "clanking"
	daemonBinary      = "clanking"
	muteLabel         = "Mute"
	mutedIcon         = "audio-volume-muted-symbolic"
	unmutedIcon       = "audio-volume-high-symbolic"
	minVolumeDB       = -60.0
	volumeStepDB      = 0.5
	volumeDigits      = 1
	volumeDebounce    = 100 * time.Millisecond
	zeroDetentDB      = 1.0
	zeroMark          = "0"
	spacing           = 6
	requestQueue      = 16
	windowMargin      = 12
	reconnectInterval = time.Second
)

type panel struct {
	volume   *gtk.Scale
	mute     *gtk.ToggleButton
	errors   *gtk.Label
	requests chan control.Request
	pending  glib.SourceHandle
	syncing  bool
	lamps    *lamps
}

func main() {
	logFile, err := statedir.OpenLog()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, nil)))
	eventsPath, err := statedir.RunPath(statedir.EventsSocket)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	app := gtk.NewApplication(appID, gio.ApplicationDefaultFlags)
	app.ConnectActivate(func() {
		p := newPanel(app, askDaemon)
		go p.watch(eventsPath)
		p.send(control.Request{Command: control.Status})
	})
	os.Exit(app.Run(os.Args))
}

func askDaemon(r control.Request) (control.Settings, error) {
	path, err := statedir.RunPath(statedir.ControlSocket)
	if err != nil {
		return control.Settings{}, err
	}
	s, _, err := transport.Control(path, r, spawnSibling)
	return s, err
}

func spawnSibling() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	return daemon.Spawn(filepath.Join(filepath.Dir(self), daemonBinary))
}

func newPanel(app *gtk.Application, ask func(control.Request) (control.Settings, error)) *panel {
	p := &panel{
		volume:   gtk.NewScaleWithRange(gtk.OrientationHorizontal, minVolumeDB, control.MaxVolumeDB, volumeStepDB),
		mute:     gtk.NewToggleButton(),
		errors:   gtk.NewLabel(""),
		requests: make(chan control.Request, requestQueue),
		lamps:    newLamps(),
	}
	go forward(p.requests, ask, func(s control.Settings, err error) { glib.IdleAdd(func() { p.show(s, err) }) })
	p.volume.SetDrawValue(true)
	p.volume.SetDigits(volumeDigits)
	p.volume.SetHExpand(true)
	p.volume.AddMark(0, gtk.PosBottom, zeroMark)
	p.volume.ConnectChangeValue(p.volumeDragged)
	p.volume.ConnectValueChanged(p.volumeChanged)
	p.mute.SetIconName(muteIcon(false))
	p.mute.SetTooltipText(muteLabel)
	p.mute.SetVAlign(gtk.AlignCenter)
	p.mute.ConnectToggled(p.muteToggled)
	controls := gtk.NewBox(gtk.OrientationHorizontal, spacing)
	controls.Append(p.mute)
	controls.Append(p.volume)
	box := gtk.NewBox(gtk.OrientationVertical, spacing)
	box.SetMarginTop(windowMargin)
	box.SetMarginBottom(windowMargin)
	box.SetMarginStart(windowMargin)
	box.SetMarginEnd(windowMargin)
	box.Append(p.lamps.row)
	box.Append(controls)
	box.Append(p.errors)
	window := gtk.NewApplicationWindow(app)
	window.SetTitle(title)
	window.SetChild(box)
	window.SetVisible(true)
	return p
}

func (p *panel) volumeDragged(scroll gtk.ScrollType, db float64) bool {
	if scroll != gtk.ScrollJump || detent(db) == db {
		return false
	}
	p.volume.SetValue(detent(db))
	return true
}

func detent(db float64) float64 {
	if math.Abs(db) < zeroDetentDB {
		return 0
	}
	return db
}

func (p *panel) volumeChanged() {
	if p.syncing {
		return
	}
	if p.pending != 0 {
		glib.SourceRemove(p.pending)
	}
	p.pending = glib.TimeoutAdd(uint(volumeDebounce.Milliseconds()), func() {
		p.pending = 0
		p.send(control.Request{Command: control.Volume, VolumeDB: p.volume.Value()})
	})
}

func (p *panel) muteToggled() {
	p.mute.SetIconName(muteIcon(p.mute.Active()))
	if p.syncing {
		return
	}
	command := control.Unmute
	if p.mute.Active() {
		command = control.Mute
	}
	p.send(control.Request{Command: command})
}

func muteIcon(muted bool) string {
	if muted {
		return mutedIcon
	}
	return unmutedIcon
}

func (p *panel) send(r control.Request) { p.requests <- r }

func forward(requests <-chan control.Request, ask func(control.Request) (control.Settings, error), reply func(control.Settings, error)) {
	for r := range requests {
		reply(ask(r))
	}
}

func (p *panel) showError(err error) {
	if err != nil {
		slog.Error("clanking-ui request failed", "err", err)
		p.errors.SetText(err.Error())
		return
	}
	p.errors.SetText("")
}

func (p *panel) show(settings control.Settings, err error) {
	p.showError(err)
	if err != nil {
		return
	}
	p.syncing = true
	p.volume.SetValue(settings.VolumeDB)
	p.mute.SetActive(settings.Muted)
	p.syncing = false
}

func (p *panel) watch(path string) {
	for {
		err := transport.Watch(path, func(line string) { glib.IdleAdd(func() { p.event(line) }) })
		slog.Warn("events stream ended", "err", err)
		time.Sleep(reconnectInterval)
	}
}

func (p *panel) event(line string) {
	if err := events.Dispatch(line, p.lamps.light, func(s control.Settings) { p.show(s, nil) }); err != nil {
		slog.Warn("unreadable event", "line", line, "err", err)
	}
}
