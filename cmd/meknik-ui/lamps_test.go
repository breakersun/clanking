package main

import (
	"strings"
	"testing"

	"meknik/synth"
)

func TestEveryCueHasALampColour(t *testing.T) {
	for _, c := range synth.Cues {
		t.Run(string(c), func(t *testing.T) {
			if _, ok := lampColours[c]; !ok {
				t.Errorf("no lamp colour for %q", c)
			}
		})
	}
}

func TestLampCSSLightsEachCueInItsColour(t *testing.T) {
	for _, c := range synth.Cues {
		t.Run(string(c), func(t *testing.T) {
			if rule := ".lamp-" + string(c) + ".lit { background-color: " + lampColours[c] + "; }"; !strings.Contains(lampCSS(), rule) {
				t.Errorf("lampCSS lacks %q", rule)
			}
		})
	}
}
