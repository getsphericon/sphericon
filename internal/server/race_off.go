//go:build !race

package server

// raceSlowdown scales the request timeout; it is 1 unless the race detector is on.
const raceSlowdown = 1
