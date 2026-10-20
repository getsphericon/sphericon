package testhelper

import (
	"testing"
	"time"
)

// receiveTimeout bounds how long a test waits for something another goroutine does.
const receiveTimeout = 30 * time.Second

// Receive waits for a value on ch and returns it; the test fails with msg when none arrives within
// receiveTimeout. The wait is the condition, not a guess: it returns as soon as the value is there.
func Receive[T any](tb testing.TB, ch <-chan T, msg string) T {
	tb.Helper()
	timer := time.NewTimer(receiveTimeout)
	defer timer.Stop()
	select {
	case v := <-ch:
		return v
	case <-timer.C:
		tb.Fatalf("%s: nothing arrived within %s", msg, receiveTimeout)
		var zero T
		return zero
	}
}

// RequireClosed fails the test with msg unless ch is closed (or holds a value) right now.
func RequireClosed[T any](tb testing.TB, ch <-chan T, msg string) {
	tb.Helper()
	select {
	case <-ch:
	default:
		tb.Fatal(msg)
	}
}
