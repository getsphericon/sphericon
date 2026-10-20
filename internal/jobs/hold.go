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

// inProgressRetryDelay is how soon to look again at a send whose claim another attempt
// currently holds: it is not a failure and not a hold, just a race to wait out.
const inProgressRetryDelay = 30 * time.Second

// HeldError reports that Outbound send put the source on hold (see
// outbound.Hold*). Nothing was consumed: the caller defers the same work.
type HeldError struct{ Reason string }

func (e *HeldError) Error() string { return "send held: " + e.Reason }

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
		return river.JobSnooze(outbound.Result{Outcome: outbound.Held}.Retry(0, 0).After)
	}
	if d, deferred := asDeferred(err); deferred {
		if d.Delay > 0 {
			return river.JobSnooze(d.Delay)
		}
		return river.JobSnooze(deferralResult(d.Wait).Retry(d.Backlog, rand.Float64()).After)
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

// deferralResult is the Outbound Result of a Deferral that waits for capacity, the input
// of the retry policy for a caller that holds only the wait.
func deferralResult(wait time.Duration) outbound.Result {
	return outbound.Result{Outcome: outbound.Deferral, Wait: wait}
}
