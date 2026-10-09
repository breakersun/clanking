package pan_test

import (
	"math"
	"testing"

	"github.com/chrchr/clanking/internal/approx"
	"github.com/chrchr/clanking/pan"
)

const (
	tolerance   = 1e-9
	clankingDir = "/home/rc/clanking"
	otherDir    = "/home/rc/lil.tel"
	spaceDir    = "/home/rc/My Projects/x"
	hardLeft    = -1.0
)

var someDirs = []string{clankingDir, otherDir, spaceDir, "/", "/tmp", "/home/rc", "/var/local/windows", "/home/rc/ia"}

func TestOfIsStable(t *testing.T) {
	if a, b := pan.Of(clankingDir), pan.Of(clankingDir); a != b {
		t.Errorf("Of twice = %v then %v, want equal", a, b)
	}
}

func TestOfStaysWithinWidth(t *testing.T) {
	for _, dir := range someDirs {
		t.Run(dir, func(t *testing.T) {
			if got := pan.Of(dir); math.Abs(got) > pan.Width {
				t.Errorf("Of(%q) = %v, want within ±%v", dir, got, pan.Width)
			}
		})
	}
}

func TestOfSeparatesDirectories(t *testing.T) {
	if a, b := pan.Of(clankingDir), pan.Of(otherDir); a == b {
		t.Errorf("Of(%q) = Of(%q) = %v, want different", clankingDir, otherDir, a)
	}
}

func TestOfCentresEmptyDirectory(t *testing.T) {
	if got := pan.Of(""); got != 0 {
		t.Errorf("Of(\"\") = %v, want 0", got)
	}
}

func TestCentreGainsAreEqual(t *testing.T) {
	left, right := pan.Gains(0)
	if !approx.Equal(left, right, tolerance) {
		t.Errorf("Gains(0) = %v, %v, want equal", left, right)
	}
}

func TestRightPanFavoursRight(t *testing.T) {
	if left, right := pan.Gains(pan.Width); right <= left {
		t.Errorf("Gains(%v) = %v, %v, want right louder", pan.Width, left, right)
	}
}

func TestGainsKeepPowerConstant(t *testing.T) {
	left, right := pan.Gains(pan.Width)
	if got := left*left + right*right; !approx.Equal(1, got, tolerance) {
		t.Errorf("left² + right² = %v, want 1", got)
	}
}
