// Package control holds the daemon's persistent mute and volume settings and the requests that change them.
package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"meknik/synth"
)

// The request commands.
const (
	Mute   = "mute"
	Unmute = "unmute"
	Volume = "volume"
	Status = "status"
	Toggle = "toggle"
	Tempo  = "tempo"
)

// The tempo range in beats per minute, and the tempo before any is set.
const (
	MinTempoBPM     = 60
	MaxTempoBPM     = 240
	DefaultTempoBPM = 120
)

// MaxVolumeDB is the loudest volume, where limited output reaches full scale.
const MaxVolumeDB = 24.0

const (
	settingsMode  = 0o644
	mutedState    = "muted"
	unmutedState  = "unmuted"
	volumeFormat  = "volume %g dB"
	tempoFormat   = "%d bpm"
	partSeparator = ", "
	statusParts   = 3
)

// ErrInvalid reports a malformed control request.
var ErrInvalid = errors.New("invalid control request")

// ErrStatus reports a status line that ParseSettings cannot read.
var ErrStatus = errors.New("invalid status line")

// Settings is the daemon's master volume and mute state.
type Settings struct {
	VolumeDB float64 `json:"volume_db"`
	Muted    bool    `json:"muted"`
	TempoBPM int     `json:"tempo_bpm"`
}

// Request is one parsed control command.
type Request struct {
	Command  string
	VolumeDB float64
	TempoBPM int
}

// Parse reads one request line.
func Parse(line string) (Request, error) {
	fields := strings.Fields(line)
	switch {
	case len(fields) == 1 && slices.Contains([]string{Mute, Unmute, Status, Toggle}, fields[0]):
		return Request{Command: fields[0]}, nil
	case len(fields) == 2 && fields[0] == Volume:
		db, err := strconv.ParseFloat(fields[1], 64)
		if err == nil && !math.IsNaN(db) && !math.IsInf(db, 0) && db <= MaxVolumeDB {
			return Request{Command: Volume, VolumeDB: db}, nil
		}
	case len(fields) == 2 && fields[0] == Tempo:
		bpm, err := strconv.Atoi(fields[1])
		if err == nil && bpm >= MinTempoBPM && bpm <= MaxTempoBPM {
			return Request{Command: Tempo, TempoBPM: bpm}, nil
		}
	}
	return Request{}, fmt.Errorf("%w: %q", ErrInvalid, strings.TrimSpace(line))
}

// String renders the request as one protocol line.
func (r Request) String() string {
	switch r.Command {
	case Volume:
		return fmt.Sprintf("%s %g", Volume, r.VolumeDB)
	case Tempo:
		return fmt.Sprintf("%s %d", Tempo, r.TempoBPM)
	}
	return r.Command
}

// Apply returns the settings after a request.
func (s Settings) Apply(r Request) Settings {
	switch r.Command {
	case Mute:
		s.Muted = true
	case Unmute:
		s.Muted = false
	case Toggle:
		s.Muted = !s.Muted
	case Volume:
		s.VolumeDB = r.VolumeDB
	case Tempo:
		s.TempoBPM = r.TempoBPM
	}
	return s
}

// Gain returns the linear master gain, which is zero when muted.
func (s Settings) Gain() float64 {
	if s.Muted {
		return 0
	}
	return synth.Amplitude(s.VolumeDB)
}

// String reports the settings as one status line.
func (s Settings) String() string {
	state := unmutedState
	if s.Muted {
		state = mutedState
	}
	return strings.Join([]string{fmt.Sprintf(volumeFormat, s.VolumeDB), state, fmt.Sprintf(tempoFormat, s.TempoBPM)}, partSeparator)
}

// ParseSettings reads a status line written by Settings.String.
func ParseSettings(line string) (Settings, error) {
	var s Settings
	parts := strings.Split(line, partSeparator)
	if len(parts) != statusParts || !slices.Contains([]string{mutedState, unmutedState}, parts[1]) {
		return Settings{}, fmt.Errorf("%w: %q", ErrStatus, line)
	}
	_, volumeErr := fmt.Sscanf(parts[0], volumeFormat, &s.VolumeDB)
	_, tempoErr := fmt.Sscanf(parts[2], tempoFormat, &s.TempoBPM)
	if volumeErr != nil || tempoErr != nil {
		return Settings{}, fmt.Errorf("%w: %q", ErrStatus, line)
	}
	s.Muted = parts[1] == mutedState
	return s, nil
}

// Controller serializes requests, persisting and applying each change.
type Controller struct {
	mu       sync.Mutex
	settings Settings
	save     func(Settings) error
	apply    func(Settings)
}

// NewController returns a controller starting from initial settings.
func NewController(initial Settings, save func(Settings) error, apply func(Settings)) *Controller {
	return &Controller{settings: initial, save: save, apply: apply}
}

// Handle applies a request, persists and applies any change, and returns the resulting settings.
func (c *Controller) Handle(r Request) (Settings, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := c.settings.Apply(r)
	if next == c.settings {
		return next, nil
	}
	if err := c.save(next); err != nil {
		return c.settings, err
	}
	c.settings = next
	c.apply(next)
	return next, nil
}

// Load reads saved settings over the defaults.
func Load(path string) (Settings, error) {
	s := Settings{TempoBPM: DefaultTempoBPM}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(data, &s)
}

// Save writes settings atomically.
func Save(path string, s Settings) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	staging := path + ".tmp"
	if err := os.WriteFile(staging, data, settingsMode); err != nil {
		return err
	}
	return os.Rename(staging, path)
}
