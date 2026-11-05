//go:build !race

package emaildoc

// raceSlowdown scales the limits' durations; it is 1 unless the race detector is on.
const raceSlowdown = 1
