package main

import (
	"strings"
	"testing"

	"github.com/chrchr/clanking/synth"
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
			if rule := ".lamp-" + string(c) + ".lit { background-color: " + lampColours[c] + ";"; !strings.Contains(lampCSS(), rule) {
				t.Errorf("lampCSS lacks %q", rule)
			}
		})
	}
}

func TestLampCSSGlowsEachLitCueInItsColour(t *testing.T) {
	for _, c := range synth.Cues {
		t.Run(string(c), func(t *testing.T) {
			if glow := "0 0 8px 2px alpha(" + lampColours[c] + ", 0.85); }"; !strings.Contains(lampCSS(), glow) {
				t.Errorf("lampCSS lacks %q", glow)
			}
		})
	}
}

func TestLampCSSDimsEachUnlitCue(t *testing.T) {
	for _, c := range synth.Cues {
		t.Run(string(c), func(t *testing.T) {
			if rule := ".lamp-" + string(c) + " { background-color: shade(" + lampColours[c] + ", 0.32); }"; !strings.Contains(lampCSS(), rule) {
				t.Errorf("lampCSS lacks %q", rule)
			}
		})
	}
}

func TestLampCSSFadesOverFourHundredMilliseconds(t *testing.T) {
	if fade := "transition: background-color 400ms ease-out, box-shadow 400ms ease-out;"; !strings.Contains(lampCSS(), fade) {
		t.Errorf("lampCSS lacks %q", fade)
	}
}

func TestLampCSSRoundsTheBezelledLamp(t *testing.T) {
	if radius := "border-radius: 9px; border: 2px solid"; !strings.Contains(lampCSS(), radius) {
		t.Errorf("lampCSS lacks %q", radius)
	}
}
