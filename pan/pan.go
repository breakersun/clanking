// Package pan places each project's sounds in the stereo field.
package pan

import (
	"hash/fnv"
	"math"
)

// Width is the furthest a project sits from centre, where 1 is one speaker only.
const Width = 0.7

// Channels is the number of interleaved output channels.
const Channels = 2

const quarterTurn = math.Pi / 4

// Of returns the pan of a working directory, in [-Width, +Width] with negative left; an empty one is centre.
func Of(cwd string) float64 {
	if cwd == "" {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(cwd))
	return Width * (2*float64(h.Sum64())/math.MaxUint64 - 1)
}

// Gains returns the constant-power left and right gains for pan p in [-1, +1].
func Gains(p float64) (left, right float64) {
	theta := (p + 1) * quarterTurn
	return math.Cos(theta), math.Sin(theta)
}
