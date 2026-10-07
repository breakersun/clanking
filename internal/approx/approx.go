// Package approx compares floating-point results in tests.
package approx

import "math"

// Equal reports whether got is within a relative tolerance of want.
func Equal(want, got, tolerance float64) bool {
	return math.Abs(got-want) <= tolerance*math.Abs(want)
}
