// Command meknik-ui is a GTK 4 window that sets the meknik daemon's volume and mute.
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

	"meknik/control"
	"meknik/daemon"
	"meknik/selftest"
	"meknik/statedir"
	"meknik/transport"
)

const (
	appID          = "tel.lil.meknik"
	title          = "meknik"
	daemonBinary   = "meknik"
	muteLabel      = "Mute"
	selfTestLabel  = "Self-test"
	minVolumeDB    = -60.0
	volumeStepDB   = 0.5
	volumeDigits   = 1
	volumeDebounce = 100 * time.Millisecond
	zeroDetentDB   = 1.0
	zeroMark       = "0"
	spacing        = 6
	requestQueue   = 16
)

type panel struct {
	volume   *gtk.Scale
	mute     *gtk.ToggleButton
	selfTest *gtk.Button
	errors   *gtk.Label
	requests chan control.Request
	test     func() error
	pending  glib.SourceHandle
	syncing  bool
}

func main() {
	logFile, err := statedir.OpenLog()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, nil)))
	app := gtk.NewApplication(appID, gio.ApplicationDefaultFlags)
	app.ConnectActivate(func() {
		newPanel(app, askDaemon, func() error { return selftest.RunDaemon(spawnSibling) }).send(control.Request{Command: control.Status})
	})
	os.Exit(app.Run(os.Args))
}

func askDaemon(r control.Request) (control.Settings, error) {
	path, err := statedir.RunPath(statedir.ControlSocket)
	if err != nil {
		return control.Settings{}, err
	}
	return transport.Control(path, r, spawnSibling)
}

func spawnSibling() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	return daemon.Spawn(filepath.Join(filepath.Dir(self), daemonBinary))
}

func newPanel(app *gtk.Application, ask func(control.Request) (control.Settings, error), test func() error) *panel {
	p := &panel{
		volume:   gtk.NewScaleWithRange(gtk.OrientationHorizontal, minVolumeDB, control.MaxVolumeDB, volumeStepDB),
		mute:     gtk.NewToggleButtonWithLabel(muteLabel),
		errors:   gtk.NewLabel(""),
		selfTest: gtk.NewButtonWithLabel(selfTestLabel),
		requests: make(chan control.Request, requestQueue),
		test:     test,
	}
	go forward(p.requests, ask, func(s control.Settings, err error) { glib.IdleAdd(func() { p.show(s, err) }) })
	p.volume.SetDrawValue(true)
	p.volume.SetDigits(volumeDigits)
	p.volume.SetHExpand(true)
	p.volume.AddMark(0, gtk.PosBottom, zeroMark)
	p.volume.ConnectChangeValue(p.volumeDragged)
	p.volume.ConnectValueChanged(p.volumeChanged)
	p.mute.ConnectToggled(p.muteToggled)
	p.selfTest.ConnectClicked(p.runSelfTest)
	box := gtk.NewBox(gtk.OrientationVertical, spacing)
	box.Append(p.volume)
	box.Append(p.mute)
	box.Append(p.selfTest)
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
	if p.syncing {
		return
	}
	command := control.Unmute
	if p.mute.Active() {
		command = control.Mute
	}
	p.send(control.Request{Command: command})
}

func (p *panel) runSelfTest() {
	p.selfTest.SetSensitive(false)
	go func() {
		err := p.test()
		glib.IdleAdd(func() {
			p.selfTest.SetSensitive(true)
			p.showError(err)
		})
	}()
}

func (p *panel) send(r control.Request) { p.requests <- r }

func forward(requests <-chan control.Request, ask func(control.Request) (control.Settings, error), reply func(control.Settings, error)) {
	for r := range requests {
		reply(ask(r))
	}
}

func (p *panel) showError(err error) {
	if err != nil {
		slog.Error("meknik-ui request failed", "err", err)
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
