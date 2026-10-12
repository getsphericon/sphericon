package jobs

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/outbound"
)

// holdRetryDelay is how long a held send waits before asking Outbound send again.
// A hold (Workspace suspension, an unverified Sending domain, no Integration) is
// reversible and not the recipient's fault, so the job is snoozed rather than
// failed: River's JobSnooze does not count against MaxAttempts, so a long hold
// never exhausts a recipient's retry budget (ADR 0015).
const holdRetryDelay = 15 * time.Minute

// inProgressRetryDelay is how soon to look again at a send whose claim another attempt
// currently holds: it is not a failure and not a hold, just a race to wait out.
const inProgressRetryDelay = 30 * time.Second

// HeldError reports that Outbound send put the source on hold (see
// outbound.Hold*). Nothing was consumed: the caller defers the same work.
type HeldError struct{ Reason string }

func (e *HeldError) Error() string { return "send held: " + e.Reason }

// deferralFloor is the shortest a Deferral waits: a limit that frees capacity in
// milliseconds must not turn into a hot retry loop.
const deferralFloor = time.Second

// deferralCeiling bounds a Deferral's snooze so a raised limit is noticed within the hour.
const deferralCeiling = time.Hour

// deferralJitter is the share of the delay added at random so a wave of deferred
// jobs does not wake in the same instant.
const deferralJitter = 0.2

// DeferredError reports that Outbound send found the Integration's Send rate limit
// spent (outbound.Deferral). Nothing was consumed: the caller retries the same work
// after Wait, and a Broadcast recipient additionally queues behind Backlog others.
type DeferredError struct {
	// Wait is how long until capacity returns for one more message.
	Wait time.Duration
	// Backlog is how many recipients of the same Broadcast are still pending, awake (not
	// themselves sleeping on a Deferral) and ahead of
	// this one; each needs a token of its own before this one gets its turn.
	Backlog int
	// Delay, when set, is the snooze already chosen for this deferral (it was recorded
	// on the recipient so the backlog of later jobs can tell it is asleep).
	Delay time.Duration
}

func (e *DeferredError) Error() string { return "send deferred: send rate limit spent" }

// asDeferred returns the DeferredError in err's chain, if any.
func asDeferred(err error) (*DeferredError, bool) {
	var d *DeferredError
	return d, errors.As(err, &d)
}

// deferralDelay is how long a Deferral waits before the job asks again: the wait
// until capacity returns, scaled by the backlog ahead (each recipient ahead needs a
// token of its own first), so the job wakes about when its turn can come instead of
// every job waking each second. The result is floored at deferralFloor and capped at
// deferralCeiling, plus up to deferralJitter of random spread (jitter is in [0, 1)).
func deferralDelay(wait time.Duration, backlog int, jitter float64) time.Duration {
	base := min(max(wait*time.Duration(1+backlog), deferralFloor), deferralCeiling)
	return base + time.Duration(float64(base)*deferralJitter*jitter)
}

// asHeld returns the HeldError in err's chain, if any.
func asHeld(err error) (*HeldError, bool) {
	var h *HeldError
	return h, errors.As(err, &h)
}

// isDeferrable reports whether err means "try again later" rather than "this attempt
// failed": a hold on the source, or a live claim held by another attempt.
func isDeferrable(err error) bool {
	_, held := asHeld(err)
	_, deferred := asDeferred(err)
	return held || deferred || errors.Is(err, outbound.ErrInProgress)
}

// snoozeIfDeferrable converts a deferrable error into the River snooze for the job
// (which does not count against MaxAttempts), passing any other error through.
// Workers call it on the error from their pure function.
func snoozeIfDeferrable(err error) error {
	if _, held := asHeld(err); held {
		return river.JobSnooze(holdRetryDelay)
	}
	if d, deferred := asDeferred(err); deferred {
		if d.Delay > 0 {
			return river.JobSnooze(d.Delay)
		}
		return river.JobSnooze(deferralDelay(d.Wait, d.Backlog, rand.Float64()))
	}
	if errors.Is(err, outbound.ErrInProgress) {
		return river.JobSnooze(inProgressRetryDelay)
	}
	return err
}

// setBroadcastHold records (or, with reason "", clears) why a broadcast is held.
// Best effort: the hold itself is enforced by the send path, this is the visible
// state, so a failed write must not mask the real outcome.
func setBroadcastHold(ctx context.Context, s *ent.Scoped, broadcastID int64, reason string) {
	upd := s.Broadcast().UpdateOneID(broadcastID)
	if reason == "" {
		upd.ClearHoldReason()
	} else {
		upd.SetHoldReason(reason)
	}
	_ = upd.Exec(ctx)
}
