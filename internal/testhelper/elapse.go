package testhelper

import "time"

// Elapse lets d of the fake clock pass inside a testing/synctest bubble: the bubble advances
// its clock as soon as every goroutine in it is blocked, so this returns at once in real time.
// It is the one way a test moves time forward. `time.Sleep` is banned (forbidigo): outside a
// bubble a sleep waits on the wall clock and makes the test slow and flaky, so wait on a
// condition instead.
func Elapse(d time.Duration) {
	<-time.After(d)
}
