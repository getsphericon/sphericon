package jobs

import (
	"errors"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/outbound"
)

// inProgressRetryDelay is how soon to look again at a send whose claim another attempt
// currently holds: it is not a failure and not a hold, just a race to wait out.
const inProgressRetryDelay = 30 * time.Second

// asDeferred returns the DeferredError in err's chain, if any.
func asDeferred(err error) (*broadcasts.DeferredError, bool) {
	var d *broadcasts.DeferredError
	return d, errors.As(err, &d)
}

// asHeld returns the HeldError in err's chain, if any.
func asHeld(err error) (*broadcasts.HeldError, bool) {
	var h *broadcasts.HeldError
	return h, errors.As(err, &h)
}

// snoozeIfDeferrable converts a deferrable error into the River snooze for the job
// (which does not count against MaxAttempts), passing any other error through.
// Workers call it on the error from their pure function.
func snoozeIfDeferrable(err error) error {
	if _, held := asHeld(err); held {
		retry := outbound.HoldRetry()
		return river.JobSnooze(retry.After)
	}
	if d, deferred := asDeferred(err); deferred {
		return river.JobSnooze(d.Delay)
	}
	if errors.Is(err, outbound.ErrInProgress) {
		return river.JobSnooze(inProgressRetryDelay)
	}
	return err
}
