package broadcasts

import (
	"context"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/internal/lifecycle"
)

// Finalize flips a Broadcast to "sent" once none of its recipients' sends is still pending.
// The finalize move (sending -> sent, a compare-and-swap) makes concurrent finalizers (one per
// per-recipient job, or an Erasure that removed the last pending recipient) a no-op
// after the first, as is a Broadcast that is not sending at all, so sent_at is set exactly once.
func Finalize(ctx context.Context, s *ent.Scoped, broadcastID int64) error {
	pending, err := s.BroadcastRecipient().Query().
		Where(broadcastrecipient.BroadcastID(broadcastID), Pending()).
		Exist(ctx)
	if err != nil {
		return err
	}
	if pending {
		return nil // not all recipients resolved yet
	}

	err = fire(ctx, s, broadcastID, triggerFinalize, func(u *ent.BroadcastScopedUpdate) {
		u.SetSentAt(time.Now()).ClearHoldReason()
	})
	if errors.Is(err, lifecycle.ErrInvalidTransition) || errors.Is(err, ErrNotFound) {
		return nil // not sending (or gone): another finalizer won, or there is nothing to settle
	}
	return err
}
