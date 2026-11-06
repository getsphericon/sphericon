//go:build race

package emaildoc

// raceSlowdown scales the limits' durations: the JavaScript runtime runs several times slower
// under the race detector, and a render that is well inside its limit otherwise would be stopped.
const raceSlowdown = 10
