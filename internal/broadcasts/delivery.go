package broadcasts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/outbound"
)

// HeldError reports that Outbound send put the source on hold (see outbound.Hold*).
// Nothing was consumed: the caller defers the same work.
type HeldError struct{ Reason string }

func (e *HeldError) Error() string { return "send held: " + e.Reason }

// DeferredError reports that Outbound send found the Integration's Send rate limit spent
// (outbound.Deferral). Nothing was consumed: the caller retries the same work after Delay.
type DeferredError struct {
	// Delay is the wait the Outbound retry policy chose for this deferral (it was recorded on
	// the recipient so the backlog of later recipients can tell it is asleep).
	Delay time.Duration
}

func (e *DeferredError) Error() string { return "send deferred: send rate limit spent" }

// Delivery is the send side of the Broadcast lifecycle: one recipient at a time through
// Outbound send, the recipient's status, the hold reason and the final settling of the
// Broadcast. It holds no client: every call receives the Workspace-scoped client.
type Delivery struct {
	out *outbound.Module
}

// NewDelivery builds the delivery over the Outbound send module.
func NewDelivery(out *outbound.Module) *Delivery { return &Delivery{out: out} }

// recipient triggers (ADR 0046): only a pending recipient can leave.
type recipientTrigger string

const (
	recipientSent    recipientTrigger = "sent"
	recipientSkipped recipientTrigger = "skipped"
	recipientFailed  recipientTrigger = "failed"
)

// recipientMachine is the BroadcastRecipient transition table: pending to sent, skipped or
// failed, each a compare-and-swap on the status read. set carries the columns the move writes.
func recipientMachine(s *ent.Scoped, id int64, set func(*ent.BroadcastRecipientScopedUpdate)) *stateless.StateMachine {
	m := lifecycle.New(lifecycle.Storage[broadcastrecipient.Status]{
		Read: func(ctx context.Context) (broadcastrecipient.Status, error) {
			r, err := s.BroadcastRecipient().Get(ctx, id)
			if err != nil {
				return "", err
			}
			return r.Status, nil
		},
		Write: func(ctx context.Context, from, to broadcastrecipient.Status) (bool, error) {
			u := s.BroadcastRecipient().Update().
				Where(broadcastrecipient.ID(id), broadcastrecipient.StatusEQ(from)).
				SetStatus(to)
			if set != nil {
				set(u)
			}
			n, err := u.Save(ctx)
			return n > 0, err
		},
	})
	m.Configure(broadcastrecipient.StatusPending).
		Permit(recipientSent, broadcastrecipient.StatusSent).
		Permit(recipientSkipped, broadcastrecipient.StatusSkipped).
		Permit(recipientFailed, broadcastrecipient.StatusFailed)
	m.Configure(broadcastrecipient.StatusSent)
	m.Configure(broadcastrecipient.StatusSkipped)
	m.Configure(broadcastrecipient.StatusFailed)
	return m
}

// SendRecipient hands one recipient's email to Outbound send and maps the Outcome onto the
// recipient, then settles the Broadcast if that was its last pending recipient. It is
// idempotent: a recipient already decided is left alone, and the Outbound idempotency key
// makes a retry after a committed send replay the recorded result.
//
// A returned error is for the adapter to retry: *HeldError (the source is on hold),
// *DeferredError (its Send rate limit is spent), outbound.ErrInProgress (a live claim) or a
// fault; none consumed the recipient. lifecycle.ErrInvalidTransition means the recipient was
// decided meanwhile: a stale job, finished without a retry.
func (d *Delivery) SendRecipient(ctx context.Context, s *ent.Scoped, recipientID int64) error {
	rec, err := s.BroadcastRecipient().Get(ctx, recipientID)
	if ent.IsNotFound(err) {
		return nil // erased with its Contact (ADR 0021) after the job was queued: nothing to send
	}
	if err != nil {
		return fmt.Errorf("load recipient %d: %w", recipientID, err)
	}
	if rec.Status == broadcastrecipient.StatusPending {
		if err := d.deliver(ctx, s, rec); err != nil {
			return err
		}
	}
	return Finalize(ctx, s, rec.BroadcastID)
}

func (d *Delivery) deliver(ctx context.Context, s *ent.Scoped, rec *ent.BroadcastRecipient) error {
	b, err := s.Broadcast().Get(ctx, rec.BroadcastID)
	if err != nil {
		return fmt.Errorf("load broadcast %d: %w", rec.BroadcastID, err)
	}
	c, err := s.Contact().Get(ctx, rec.ContactID)
	if ent.IsNotFound(err) {
		return nil // the Contact was erased (ADR 0021): never send to it
	}
	if err != nil {
		return fmt.Errorf("load contact %d: %w", rec.ContactID, err)
	}
	// Guard: the audience filters EmailNotNil, but a contact could lose its email
	// between plan and send. Terminal: the row can never send.
	if c.Email == nil {
		return d.fail(ctx, s, rec, errNoEmail)
	}

	res, err := d.out.Send(ctx, s, outbound.BroadcastRecipient(b, rec, c))
	if err != nil {
		return err
	}
	if retry := res.Retry(0); retry.Wanted {
		return d.retryLater(ctx, s, b, rec, res)
	}
	return d.apply(ctx, s, rec, res)
}

// errNoEmail is the reason of a recipient whose Contact has no address to send to.
var errNoEmail = errors.New("contact has no email")

// fail ends a recipient's send as Failed with cause: Outbound send records the one message,
// and the recipient follows what that message says (a send that already went out stays sent).
// outbound.ErrInProgress is a live claim: the caller waits and asks again.
func (d *Delivery) fail(ctx context.Context, s *ent.Scoped, rec *ent.BroadcastRecipient, cause error) error {
	res, err := d.out.FailRecipient(ctx, s, rec, cause)
	if err != nil {
		return err
	}
	return d.apply(ctx, s, rec, res)
}

// apply maps a decided Outbound outcome onto the recipient.
func (d *Delivery) apply(ctx context.Context, s *ent.Scoped, rec *ent.BroadcastRecipient, res outbound.Result) error {
	switch res.Outcome {
	case outbound.Sent:
		// A send went through: the hold, if any, has lifted.
		if err := setHold(ctx, s, rec.BroadcastID, ""); err != nil {
			return err
		}
		return moveRecipient(ctx, s, rec.ID, recipientSent, func(u *ent.BroadcastRecipientScopedUpdate) {
			withMessage(u, res).SetSentAt(time.Now())
		})
	case outbound.Skipped:
		return moveRecipient(ctx, s, rec.ID, recipientSkipped, func(u *ent.BroadcastRecipientScopedUpdate) {
			withMessage(u, res).SetError(res.Reason)
		})
	default: // outbound.Failed
		return moveRecipient(ctx, s, rec.ID, recipientFailed, func(u *ent.BroadcastRecipientScopedUpdate) {
			withMessage(u, res).SetError(res.Reason)
		})
	}
}

func withMessage(u *ent.BroadcastRecipientScopedUpdate, res outbound.Result) *ent.BroadcastRecipientScopedUpdate {
	if res.MessageID != 0 {
		u.SetOutboundMessageID(res.MessageID)
	}
	return u
}

func moveRecipient(ctx context.Context, s *ent.Scoped, id int64, t recipientTrigger, set func(*ent.BroadcastRecipientScopedUpdate)) error {
	return recipientMachine(s, id, set).FireCtx(ctx, t)
}

// retryLater is the recipient's turn when Outbound send wants a retry (a Hold or a
// Deferral): nothing was consumed, so the recipient stays pending.
func (d *Delivery) retryLater(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, rec *ent.BroadcastRecipient, res outbound.Result) error {
	if res.Outcome == outbound.Held {
		// Best effort: the hold is enforced by the send path and the caller retries on the
		// HeldError either way, so a failed write only leaves the reason unrecorded.
		_ = setHold(ctx, s, b.ID, res.Reason)
		return &HeldError{Reason: res.Reason}
	}
	// The Integration is busy, not blocked: the Broadcast stays sending with no hold
	// reason and this recipient stays pending, retried after the wait.
	now := time.Now()
	ahead, cerr := s.BroadcastRecipient().Query().
		Where(
			broadcastrecipient.BroadcastID(rec.BroadcastID),
			broadcastrecipient.StatusEQ(broadcastrecipient.StatusPending),
			broadcastrecipient.IDLT(rec.ID),
			// Only the awake ones queue for a token; a recipient asleep on its own
			// Deferral is not ahead of this one yet.
			broadcastrecipient.Or(
				broadcastrecipient.DeferredUntilIsNil(),
				broadcastrecipient.DeferredUntilLTE(now),
			),
		).Count(ctx)
	if cerr != nil {
		ahead = 0 // the estimate only spreads retries; never fail the send over it
	}
	retry := res.Retry(ahead)
	// Best effort, like the count: it only sharpens later recipients' backlog.
	upd := s.BroadcastRecipient().UpdateOneID(rec.ID).SetDeferredUntil(now.Add(retry.After))
	if res.MessageID != 0 {
		upd.SetOutboundMessageID(res.MessageID)
	}
	_ = upd.Exec(ctx)
	return &DeferredError{Delay: retry.After}
}

// GiveUp records the terminal failure of a recipient whose last attempt failed with cause:
// Outbound send ends its message as Failed with the cause as its reason (a send that already
// reached the provider stays sent), and the Broadcast settles if none is left pending. A
// recipient already decided is lifecycle.ErrInvalidTransition and keeps its status.
//
// outbound.ErrInProgress means another attempt still holds a live claim and may yet reach the
// provider: nothing is failed, and the adapter waits out the lease and gives up again.
func (d *Delivery) GiveUp(ctx context.Context, s *ent.Scoped, recipientID int64, cause error) error {
	rec, err := s.BroadcastRecipient().Get(ctx, recipientID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := d.fail(ctx, s, rec, cause); err != nil {
		return err
	}
	return Finalize(ctx, s, rec.BroadcastID)
}

// setHold records (or, with reason "", clears) why a Broadcast is held. It is the only
// writer of the hold reason.
func setHold(ctx context.Context, s *ent.Scoped, broadcastID int64, reason string) error {
	upd := s.Broadcast().UpdateOneID(broadcastID)
	if reason == "" {
		upd.ClearHoldReason()
	} else {
		upd.SetHoldReason(reason)
	}
	return upd.Exec(ctx)
}

// IsRetryLater reports whether err asks the adapter to try again rather than record a
// failure: a hold, a Deferral or a live claim.
func IsRetryLater(err error) bool {
	var held *HeldError
	var deferred *DeferredError
	return errors.As(err, &held) || errors.As(err, &deferred) || errors.Is(err, outbound.ErrInProgress)
}
