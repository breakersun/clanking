package main

import (
	"math"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const (
	knobSize          = 48
	knobStart         = 0.75 * math.Pi
	knobSweep         = 1.5 * math.Pi
	fullTurn          = 2 * math.Pi
	knobPixelsPerStep = 1.0
	knobTrackWidth    = 3.0
	knobTrackGap      = 4.0
	knobBezelWidth    = 2.0
	knobCapRatio      = 0.7
	knobPointerWidth  = 2.5
	knobPointerInner  = 0.3
	knobPointerOuter  = 0.85
	knobCursor        = "ns-resize"
	knobClass         = "knob"
	knobCSS           = ".knob { color: @accent_color; }"
	knobTrackAlpha    = 0.2
	knobBodyAlpha     = 0.1
	knobBezelAlpha    = 0.35
	knobCapAlpha      = 0.08
	opaque            = 1.0
)

type knob struct {
	box       *gtk.Box
	area      *gtk.DrawingArea
	label     *gtk.Label
	format    func(int) string
	min, max  int
	value     int
	dragStart int
	changed   func()
}

func newKnob(minimum, maximum int, format func(int) string, changed func()) *knob {
	k := &knob{box: gtk.NewBox(gtk.OrientationVertical, 0), area: gtk.NewDrawingArea(), label: gtk.NewLabel(""),
		format: format, min: minimum, max: maximum, changed: changed}
	addCSS(knobCSS)
	k.area.AddCSSClass(knobClass)
	k.area.SetContentWidth(knobSize)
	k.area.SetContentHeight(knobSize)
	k.area.SetCursorFromName(knobCursor)
	k.area.SetDrawFunc(k.draw)
	drag := gtk.NewGestureDrag()
	drag.ConnectDragBegin(func(_, _ float64) { k.dragStart = k.value })
	drag.ConnectDragUpdate(func(_, dy float64) { k.adjust(k.dragged(dy)) })
	k.area.AddController(drag)
	scroll := gtk.NewEventControllerScroll(gtk.EventControllerScrollVertical | gtk.EventControllerScrollDiscrete)
	scroll.ConnectScroll(func(_, dy float64) bool {
		k.adjust(k.scrolled(dy))
		return true
	})
	k.area.AddController(scroll)
	k.box.Append(k.area)
	k.box.Append(k.label)
	k.set(minimum)
	return k
}

func (k *knob) set(value int) {
	k.value = value
	k.label.SetText(k.format(value))
	k.area.QueueDraw()
}

func (k *knob) adjust(value int) {
	if value == k.value {
		return
	}
	k.set(value)
	k.changed()
}

func (k *knob) angle() float64 {
	return knobStart + knobSweep*float64(k.value-k.min)/float64(k.max-k.min)
}

func (k *knob) clamp(value int) int { return min(max(value, k.min), k.max) }

func (k *knob) dragged(dy float64) int {
	return k.clamp(k.dragStart - int(math.Round(dy/knobPixelsPerStep)))
}

func (k *knob) scrolled(dy float64) int { return k.clamp(k.value - int(math.Round(dy))) }

func (k *knob) draw(_ *gtk.DrawingArea, cr *cairo.Context, width, height int) {
	drawKnob(cr, width, height, k.angle(), k.box.Color(), k.area.Color())
}

func drawKnob(cr *cairo.Context, width, height int, angle float64, fg, accent *gdk.RGBA) {
	cx, cy := float64(width)/2, float64(height)/2
	track := math.Min(cx, cy) - knobTrackWidth/2
	body := track - knobTrackWidth/2 - knobTrackGap
	cr.SetLineCap(cairo.LineCapRound)
	cr.SetLineWidth(knobTrackWidth)
	paint(cr, fg, knobTrackAlpha)
	cr.Arc(cx, cy, track, knobStart, knobStart+knobSweep)
	cr.Stroke()
	paint(cr, accent, opaque)
	cr.Arc(cx, cy, track, knobStart, angle)
	cr.Stroke()
	cr.Arc(cx, cy, body, 0, fullTurn)
	paint(cr, fg, knobBodyAlpha)
	cr.FillPreserve()
	cr.SetLineWidth(knobBezelWidth)
	paint(cr, fg, knobBezelAlpha)
	cr.Stroke()
	cr.Arc(cx, cy, body*knobCapRatio, 0, fullTurn)
	paint(cr, fg, knobCapAlpha)
	cr.Fill()
	dx, dy := math.Cos(angle), math.Sin(angle)
	cr.SetLineWidth(knobPointerWidth)
	paint(cr, accent, opaque)
	cr.MoveTo(cx+dx*body*knobPointerInner, cy+dy*body*knobPointerInner)
	cr.LineTo(cx+dx*body*knobPointerOuter, cy+dy*body*knobPointerOuter)
	cr.Stroke()
}

func paint(cr *cairo.Context, c *gdk.RGBA, alpha float64) {
	cr.SetSourceRGBA(float64(c.Red()), float64(c.Green()), float64(c.Blue()), float64(c.Alpha())*alpha)
}
