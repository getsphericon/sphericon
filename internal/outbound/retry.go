package outbound

import (
	"math/rand/v2"
	"time"

	"github.com/getsphericon/sphericon/internal/providerlimit"
)

// Retry says whether an Outbound Outcome wants another attempt and after how long. It is
// the one retry policy: a queue adapter turns After into its own wait (a River snooze, an
// Automation run's resume time) and keeps no delay of its own.
type Retry struct {
	// Wanted is true for a Hold and a Deferral, the reversible Outcomes; every other
	// Outcome is final.
	Wanted bool
	// After is how long to wait before asking Outbound send again; zero when not Wanted.
	After time.Duration
}

// HoldBackoff is how long a Hold waits before asking again. A Hold (Workspace suspension,
// an unverified Sending domain, no Integration) is reversible and not the recipient's
// fault, so the caller waits without spending an attempt (ADR 0015).
const HoldBackoff = 15 * time.Minute

const (
	// deferralFloor is the shortest a Deferral waits: a limit that frees capacity in
	// milliseconds must not turn into a hot retry loop.
	deferralFloor = time.Second
	// deferralCeiling bounds a Deferral's wait so a raised limit is noticed within the hour.
	deferralCeiling = time.Hour
	// deferralJitter is the share of the delay added at random so a wave of deferred jobs
	// does not wake in the same instant.
	deferralJitter = 0.2
	// sendCapBound is the longest a Deferral caused by the Send cap waits, jitter included
	// (ADR 0067): a Send cap a Staff member raises is noticed within minutes, with no job
	// rewritten. Its ceiling before jitter leaves room for the jitter under the bound.
	sendCapBound   = 10 * time.Minute
	sendCapCeiling = time.Duration(float64(sendCapBound) / (1 + deferralJitter))
)

// HoldRetry is the retry of a Hold: HoldBackoff, whatever the source.
func HoldRetry() Retry {
	return Retry{Wanted: true, After: HoldBackoff}
}

// Retry is the retry policy for this Result. backlog is how many sends of the same source
// are awake and ahead of this one (each needs a token of its own first; 0 for a source
// with no queue). The jitter of a Deferral is drawn here, the one place it is used.
//
// A Deferral waits the wait until capacity returns scaled by the backlog, floored at
// deferralFloor and capped at deferralCeiling, plus up to deferralJitter of random spread; one
// caused by the Send cap is capped so that, jitter included, it never waits past sendCapBound.
// A Hold waits HoldBackoff.
func (r Result) Retry(backlog int) Retry {
	switch r.Outcome {
	case Held:
		return HoldRetry()
	case Deferral:
		ceiling := deferralCeiling
		if r.Cause == providerlimit.CauseSendCap {
			ceiling = sendCapCeiling
		}
		return deferralRetry(r.Wait, backlog, rand.Float64(), ceiling)
	default:
		return Retry{}
	}
}

// deferralRetry is the Deferral policy with the jitter, in [0, 1), and the ceiling given.
func deferralRetry(wait time.Duration, backlog int, jitter float64, ceiling time.Duration) Retry {
	// The product saturates at the ceiling instead of overflowing int64: a long wait
	// behind a large backlog would otherwise wrap negative and wake the job after the floor.
	ahead := int64(1 + backlog)
	base := ceiling
	if int64(wait) <= int64(ceiling)/ahead {
		base = min(max(time.Duration(int64(wait)*ahead), deferralFloor), ceiling)
	}
	return Retry{Wanted: true, After: base + time.Duration(float64(base)*deferralJitter*jitter)}
}
