package synth

import (
	"maps"
	"slices"
	"time"
)

// The XDG sound theme names with motifs of their own.
const (
	DialogWarning        Cue = "dialog-warning"
	Bell                 Cue = "bell"
	AlarmClockElapsed    Cue = "alarm-clock-elapsed"
	TrashEmpty           Cue = "trash-empty"
	ServiceLogin         Cue = "service-login"
	ServiceLogout        Cue = "service-logout"
	SuspendError         Cue = "suspend-error"
	PhoneIncomingCall    Cue = "phone-incoming-call"
	PhoneOutgoingCalling Cue = "phone-outgoing-calling"
	PhoneOutgoingBusy    Cue = "phone-outgoing-busy"
	AudioVolumeChange    Cue = "audio-volume-change"
	CameraShutter        Cue = "camera-shutter"
	ScreenCapture        Cue = "screen-capture"
	NetworkEstablished   Cue = "network-connectivity-established"
	NetworkLost          Cue = "network-connectivity-lost"
	PowerPlug            Cue = "power-plug"
	PowerUnplug          Cue = "power-unplug"
	BatteryLow           Cue = "battery-low"
)

// ThemeCues lists the XDG sound theme names with motifs of their own.
var ThemeCues = []Cue{
	DialogWarning, Bell, AlarmClockElapsed, TrashEmpty, ServiceLogin, ServiceLogout, SuspendError,
	PhoneIncomingCall, PhoneOutgoingCalling, PhoneOutgoingBusy, AudioVolumeChange, CameraShutter, ScreenCapture,
	NetworkEstablished, NetworkLost, PowerPlug, PowerUnplug, BatteryLow,
}

var aliases = map[Cue]Cue{
	"complete": Done, "dialog-error": Failed, "dialog-question": Ask, "dialog-information": Noted,
	"window-attention": Permission, "message": Reply, "message-new-instant": Reply,
	"device-added": Spawn, "device-removed": Return,
}

const (
	toll      = 600 * time.Millisecond
	tollDecay = 250 * time.Millisecond
	chirp     = 40 * time.Millisecond
	burstGap  = 120 * time.Millisecond
	sigh      = 180 * time.Millisecond
	stab      = 80 * time.Millisecond
	trill     = 30 * time.Millisecond
	ringback  = 400 * time.Millisecond
	busyBeep  = 120 * time.Millisecond
	click     = 15 * time.Millisecond
	shutter   = 50 * time.Millisecond
	droop     = 150 * time.Millisecond
)

var (
	alarmBurst = []float64{7, 12, 7, 12}
	trillBurst = []float64{9, 12, 9, 12, 9, 12, 9, 12}
	scaleDown  = []float64{12, 11, 9, 7, 5, 4, 2, 0}
)

var themeScores = map[Cue]score{
	DialogWarning: {CuePeakDB, []Note{rung(9, 0), rung(4, bellGap)}},
	Bell:          {CuePeakDB, []Note{tolled(0), tolled(7)}},
	AlarmClockElapsed: {CuePeakDB, slices.Concat(
		phrase(0, chirp, alarmBurst...), phrase(4*chirp+burstGap, chirp, alarmBurst...), phrase(8*chirp+2*burstGap, chirp, alarmBurst...))},
	TrashEmpty:    {CuePeakDB, phrase(0, blip, scaleDown...)},
	ServiceLogin:  {CuePeakDB, []Note{rung(0, 0), rung(7, sigh)}},
	ServiceLogout: {CuePeakDB, []Note{rung(7, 0), rung(0, sigh)}},
	SuspendError:  {CuePeakDB, []Note{buzz(-7, 0, stab), buzz(-7, stab+chirp, stab)}},
	PhoneIncomingCall: {CuePeakDB, slices.Concat(
		phrase(0, trill, trillBurst...), phrase(8*trill+burstGap, trill, trillBurst...))},
	PhoneOutgoingCalling: {CuePeakDB, []Note{held(4, 0, ringback), held(7, 0, ringback), held(4, ringback+sigh, ringback), held(7, ringback+sigh, ringback)}},
	PhoneOutgoingBusy:    {CuePeakDB, []Note{held(-5, 0, busyBeep), held(-5, 2*busyBeep, busyBeep), held(-5, 4*busyBeep, busyBeep), held(-5, 6*busyBeep, busyBeep)}},
	AudioVolumeChange:    {ReplyPeakDB, []Note{held(7, 0, chirp)}},
	CameraShutter:        {CuePeakDB, []Note{buzz(-12, 0, click), buzz(-12, shutter, click)}},
	ScreenCapture:        {CuePeakDB, []Note{buzz(-12, 0, click), {From: 19, To: 19, Onset: shutter, Length: plink, Decay: plinkDecay, Timbre: Sine}}},
	NetworkEstablished:   {CuePeakDB, []Note{held(7, 0, bloop), held(12, bloop, bloop)}},
	NetworkLost:          {CuePeakDB, []Note{held(12, 0, bloop), held(7, bloop, bloop)}},
	PowerPlug:            {CuePeakDB, []Note{held(0, 0, bloop), held(12, bloop, bloop)}},
	PowerUnplug:          {CuePeakDB, []Note{held(12, 0, bloop), held(0, bloop, bloop)}},
	BatteryLow:           {CuePeakDB, []Note{held(2, 0, droop), held(-1, droop, droop), held(-5, 2*droop, 2*droop)}},
}

// Canonical returns the cue an XDG alias plays, or c itself when it is no alias.
func Canonical(c Cue) Cue {
	if target, ok := aliases[c]; ok {
		return target
	}
	return c
}

// Aliases lists the XDG names that play an existing cue, sorted.
func Aliases() []Cue { return slices.Sorted(maps.Keys(aliases)) }

// Known reports whether c names a cue, a theme cue, or an alias.
func Known(c Cue) bool {
	_, theme := themeScores[c]
	_, alias := aliases[c]
	return slices.Contains(Cues, c) || theme || alias
}

func phrase(onset, step time.Duration, semitones ...float64) []Note {
	notes := make([]Note, len(semitones))
	for i, st := range semitones {
		notes[i] = held(st, onset+time.Duration(i)*step, step)
	}
	return notes
}

func tolled(semitones float64) Note {
	return Note{From: semitones, To: semitones, Length: toll, Decay: tollDecay, Timbre: Sine}
}

func buzz(semitones float64, onset, length time.Duration) Note {
	return Note{From: semitones, To: semitones, Onset: onset, Length: length, Decay: sustain, Timbre: Buzzy}
}
