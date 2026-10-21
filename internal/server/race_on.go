//go:build race

package server

// raceSlowdown scales the request timeout: rendering an email document runs several times slower
// in the JavaScript runtime under the race detector (internal/emaildoc scales its own limits the
// same way), and a request well inside its limit would otherwise be answered 504.
const raceSlowdown = 10
