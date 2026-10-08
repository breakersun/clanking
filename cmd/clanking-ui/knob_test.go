package main

import (
	"math"
	"testing"
)

const (
	knobTestMin = 60
	knobTestMax = 240
)

func testKnob(value int) *knob {
	return &knob{min: knobTestMin, max: knobTestMax, value: value, dragStart: value}
}

func TestKnobAngleAtMinimumIsSweepStart(t *testing.T) {
	if got := testKnob(knobTestMin).angle(); got != knobStart {
		t.Errorf("angle() = %v, want %v", got, knobStart)
	}
}

func TestKnobAngleAtMaximumIsSweepEnd(t *testing.T) {
	if got, want := testKnob(knobTestMax).angle(), knobStart+knobSweep; math.Abs(got-want) > 1e-9 {
		t.Errorf("angle() = %v, want %v", got, want)
	}
}

func TestKnobDragUpRaisesValue(t *testing.T) {
	if got := testKnob(85).dragged(-10); got != 95 {
		t.Errorf("dragged(-10) = %d, want 95", got)
	}
}

func TestKnobDragClampsAtMaximum(t *testing.T) {
	if got := testKnob(230).dragged(-100); got != knobTestMax {
		t.Errorf("dragged(-100) = %d, want %d", got, knobTestMax)
	}
}

func TestKnobScrollDownLowersValue(t *testing.T) {
	if got := testKnob(85).scrolled(1); got != 84 {
		t.Errorf("scrolled(1) = %d, want 84", got)
	}
}

func TestKnobScrollClampsAtMinimum(t *testing.T) {
	if got := testKnob(knobTestMin).scrolled(1); got != knobTestMin {
		t.Errorf("scrolled(1) = %d, want %d", got, knobTestMin)
	}
}
