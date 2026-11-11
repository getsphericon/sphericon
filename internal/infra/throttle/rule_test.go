package throttle_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/infra/throttle"
)

func TestLoginWaitIsNoneBeforeTheThreshold(t *testing.T) {
	rule := throttle.LoginRule(5)
	for failures := range 5 {
		wait := rule.Wait(failures)
		assert.Zero(t, wait, "%d failures", failures)
	}
}

// From the threshold on, the wait doubles from 1 s per failure until it reaches the
// 15-minute cap at the 11th failure past it.
func TestLoginWaitDoublesFromOneSecondToTheCap(t *testing.T) {
	rule := throttle.LoginRule(5)
	want := []time.Duration{
		time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		32 * time.Second,
		64 * time.Second,
		128 * time.Second,
		256 * time.Second,
		512 * time.Second,
		15 * time.Minute,
	}
	for k, wait := range want {
		got := rule.Wait(5 + k)
		assert.Equal(t, wait, got, "%d failures past the threshold", k)
	}
}

// Over the whole range up to the cap and well past it, the wait is what the doubling
// schedule gives.
func TestLoginWaitAgreesWithTheDoublingScheduleOverItsRange(t *testing.T) {
	rule := throttle.LoginRule(5)
	for k := range 40 {
		got, want := rule.Wait(5+k), doublingWait(rule, 5+k)
		assert.Equal(t, want, got, "%d failures past the threshold", k)
	}
}

func TestWaitAgreesWithTheDoublingSchedule(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		base := time.Duration(rapid.Int64Range(1, 1<<40).Draw(rt, "base"))
		// Below 2^53 ns a doubled duration is exact as a float64 and cannot overflow.
		limit := time.Duration(rapid.Int64Range(int64(base), 1<<52).Draw(rt, "cap"))
		rule := throttle.Rule{
			Threshold: rapid.IntRange(1, 100).Draw(rt, "threshold"),
			Window:    time.Hour,
			Base:      base,
			Cap:       limit,
		}
		failures := rapid.IntRange(0, 100_000).Draw(rt, "failures")

		got, want := rule.Wait(failures), doublingWait(rule, failures)

		if got != want {
			rt.Fatalf("%+v after %d failures: wait %v, want %v", rule, failures, got, want)
		}
	})
}

// A budget Rule has no Base: however many slots are spent, nothing waits.
func TestWaitWithoutABaseIsNone(t *testing.T) {
	rule := throttle.ResetRule(3)
	for failures := range 20 {
		wait := rule.Wait(failures)
		assert.Zero(t, wait, "%d failures", failures)
	}
}

func TestDisabledRuleWaitIsNone(t *testing.T) {
	wait := throttle.LoginRule(0).Wait(100)
	assert.Zero(t, wait)
}

// A Cap below the Base, or none, does not let the wait grow: it stays at the Base.
func TestWaitWithoutRoomToGrowStaysAtTheBase(t *testing.T) {
	for _, limit := range []time.Duration{0, time.Second} {
		rule := throttle.Rule{Threshold: 1, Window: time.Hour, Base: time.Minute, Cap: limit}
		for failures := 1; failures < 10; failures++ {
			wait := rule.Wait(failures)
			assert.Equal(t, time.Minute, wait, "cap %v, %d failures", limit, failures)
		}
	}
}

// doublingWait is the schedule as a plain loop: none below the threshold, then the Base
// doubled per failure past it, up to the Cap.
func doublingWait(rule throttle.Rule, failures int) time.Duration {
	if failures < rule.Threshold {
		return 0
	}
	wait := rule.Base
	for range failures - rule.Threshold {
		wait *= 2
		if wait >= rule.Cap {
			return rule.Cap
		}
	}
	return wait
}
