package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"meknik/synth"
)

const (
	lampSize       = 14
	lampSpacing    = 8
	lampHold       = 40 * time.Millisecond
	lampClass      = "lamp"
	litClass       = "lit"
	lampDark       = "#1b2130"
	lampBaseRule   = ".lamp { min-width: %dpx; min-height: %dpx; border-radius: %dpx; background-color: %s; transition: background-color 200ms ease-out; }\n.lamp.lit { transition: none; }\n"
	lampColourRule = ".lamp-%s.lit { background-color: %s; }\n"
)

var lampColours = map[synth.Cue]string{
	synth.Tick:       "#ffcc99",
	synth.Spawn:      "#ff9966",
	synth.Return:     "#cc99cc",
	synth.Failed:     "#cc6666",
	synth.Permission: "#ffcc66",
	synth.Ask:        "#9999ff",
	synth.Reply:      "#99ccff",
	synth.Milestone:  "#ff9900",
	synth.Noted:      "#cc99ff",
	synth.Done:       "#99cc99",
}

type lamps struct {
	row     *gtk.Box
	lamp    map[synth.Cue]*gtk.Box
	pending map[synth.Cue]glib.SourceHandle
}

func lampCSS() string {
	var css strings.Builder
	fmt.Fprintf(&css, lampBaseRule, lampSize, lampSize, lampSize/2, lampDark)
	for _, c := range synth.Cues {
		fmt.Fprintf(&css, lampColourRule, c, lampColours[c])
	}
	return css.String()
}

func newLamps() *lamps {
	provider := gtk.NewCSSProvider()
	provider.LoadFromString(lampCSS())
	gtk.StyleContextAddProviderForDisplay(gdk.DisplayGetDefault(), provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
	l := &lamps{row: gtk.NewBox(gtk.OrientationHorizontal, lampSpacing), lamp: map[synth.Cue]*gtk.Box{}, pending: map[synth.Cue]glib.SourceHandle{}}
	l.row.SetHAlign(gtk.AlignCenter)
	for _, c := range synth.Cues {
		lamp := gtk.NewBox(gtk.OrientationHorizontal, 0)
		lamp.AddCSSClass(lampClass)
		lamp.AddCSSClass(lampClass + "-" + string(c))
		lamp.SetTooltipText(string(c))
		l.lamp[c] = lamp
		l.row.Append(lamp)
	}
	return l
}

func (l *lamps) light(c synth.Cue) {
	lamp, ok := l.lamp[c]
	if !ok {
		return
	}
	if l.pending[c] != 0 {
		glib.SourceRemove(l.pending[c])
	}
	lamp.AddCSSClass(litClass)
	l.pending[c] = glib.TimeoutAdd(uint(lampHold.Milliseconds()), func() {
		l.pending[c] = 0
		lamp.RemoveCSSClass(litClass)
	})
}
