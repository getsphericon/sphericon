package broadcasts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
	"github.com/getsphericon/sphericon/internal/kernel/holdreason"
)

// HeldError reports that Outbound send put the source on hold, for Reason.
// Nothing was consumed: the caller defers the same work.
type HeldError struct{ Reason holdreason.Reason }

func (e *HeldError) Error() string { return "send held: " + string(e.Reason) }

// DeferredError reports that Outbound send found a send limit spent (outboundmessages.Deferral): the
// Integration's Provider limit, the Workspace's Send cap or the provider's own. Nothing was
// consumed: the caller retries the same work after Delay.
type DeferredError struct {
	// Delay is the wait the Outbound retry policy chose for this deferral (it was recorded on
	// the recipient so the backlog of later recipients can tell it is asleep).
	Delay time.Duration
	// Cause is the limit the deferral waits for.
	Cause deferralcause.Cause
}

func (e *DeferredError) Error() string { return "send deferred: " + string(e.Cause) + " spent" }

// Delivery is the send side of the Broadcast lifecycle: one recipient at a time through
// Outbound send, the hold reason and the final settling of the Broadcast. A recipient has no
// delivery state of its own: how its send ended is its Outbound message's (ADR 0015), and it is
// pending until then. It holds no client: every call receives the Workspace-scoped client.
type Delivery struct {
	out       *outboundmessages.Module
	sched     Scheduler
	audiences audiences
}

// NewDelivery builds the delivery over the Outbound send module, whose clock it reads, and the
// scheduler it hands a planned Broadcast's sends to.
func NewDelivery(out *outboundmessages.Module, sched Scheduler) *Delivery {
	return &Delivery{out: out, sched: sched, audiences: newAudiences(out)}
}

// SendRecipient hands one recipient's email to Outbound send, then settles the Broadcast if
// that was its last pending recipient. It is idempotent: a recipient already decided is left
// alone (a stale job is just that), and the Outbound idempotency key makes a retry after a
// committed send replay the recorded result. A recipient of a cancelled Broadcast that has no
// message gets none (ADR 0150): the job is done, and a claim that raced Cancel is withdrawn.
//
// A returned error is for the job to retry (DeliverRecipient reads it): *HeldError (the source is on hold),
// *DeferredError (a send limit is spent), outboundmessages.ErrInProgress (a live claim) or a
// fault; none consumed the recipient.
func (d *Delivery) SendRecipient(ctx context.Context, s *ent.Scoped, recipientID int64) error {
	rec, err := s.BroadcastRecipient().Get(ctx, recipientID)
	if ent.IsNotFound(err) {
		return nil // erased with its Contact (ADR 0021) after the job was queued: nothing to send
	}
	if err != nil {
		return fmt.Errorf("load recipient %d: %w", recipientID, err)
	}
	pending, err := s.BroadcastRecipient().Query().
		Where(broadcastrecipient.ID(rec.ID), Pending()).Exist(ctx)
	if err != nil {
		return fmt.Errorf("read recipient %d: %w", recipientID, err)
	}
	if pending {
		if err := d.deliver(ctx, s, rec); err != nil {
			return err
		}
	}
	return finalize(ctx, s, rec.BroadcastID, d.out.Now())
}

func (d *Delivery) deliver(ctx context.Context, s *ent.Scoped, rec *ent.BroadcastRecipient) error {
	b, err := s.Broadcast().Get(ctx, rec.BroadcastID)
	if err != nil {
		return fmt.Errorf("load broadcast %d: %w", rec.BroadcastID, err)
	}
	if stopped, err := cancelledBefore(ctx, s, b, rec); err != nil || stopped {
		return err
	}
	c, err := s.Contact().Get(ctx, rec.ContactID)
	if ent.IsNotFound(err) {
		return nil // the Contact was erased (ADR 0021): never send to it
	}
	if err != nil {
		return fmt.Errorf("load contact %d: %w", rec.ContactID, err)
	}
	// Guard: the email audience filters EmailNotNil, but a contact could lose its email between plan
	// and send. Terminal: the row can never send. A Channel of Bindings has no such guard: a Contact
	// that lost its last Binding is Skipped no_destination by Outbound send.
	if b.Channel == channels.Email && c.Email == nil {
		return d.fail(ctx, s, rec, errNoEmail)
	}

	req, err := outboundmessages.BroadcastRecipient(b, rec, c)
	if err != nil {
		return err
	}
	req.Stopped = cancelledAt(b.ID)
	res, err := d.out.Send(ctx, s, req)
	if err != nil {
		return err
	}
	if retry := res.Retry(0); retry.Wanted {
		return d.retryLater(ctx, s, b, rec, res)
	}
	return d.settle(ctx, s, rec, res)
}

// cancelledBefore reports whether the recipient's Broadcast b was cancelled before anything was
// claimed for it (ADR 0150): no message is created for it, sent or failed. A claim written before
// Cancel goes on, so a recipient that has one is not stopped here. It reads b as loaded: a Cancel
// that lands after it is caught at the claim (Request.Stopped).
func cancelledBefore(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, rec *ent.BroadcastRecipient) (bool, error) {
	if b.Status != broadcast.StatusCancelled {
		return false, nil
	}
	none, err := s.BroadcastRecipient().Query().Where(broadcastrecipient.ID(rec.ID), unclaimed()).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("read recipient %d: %w", rec.ID, err)
	}
	return none, nil
}

// cancelledAt is the question a recipient's message asks of Broadcast id once its row is written
// (Request.Stopped, outboundmessages.Failure.Stopped): when it was cancelled, nil while it is not,
// so a write that raced Cancel loses.
func cancelledAt(id int64) outboundmessages.SourceStop {
	return func(ctx context.Context, s *ent.Scoped) (*time.Time, error) {
		b, err := s.Broadcast().Query().Where(broadcast.ID(id)).Select(broadcast.FieldStatus, broadcast.FieldCancelledAt).Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("read broadcast %d: %w", id, err)
		}
		if b.Status != broadcast.StatusCancelled {
			return nil, nil
		}
		return b.CancelledAt, nil
	}
}

// stillSending reports whether Broadcast id still sends: the question the plan asks once its rows
// are written, so a plan that Cancel overtook schedules nothing.
func stillSending(ctx context.Context, s *ent.Scoped, id int64) (bool, error) {
	return s.Broadcast().Query().Where(broadcast.ID(id), broadcast.StatusEQ(broadcast.StatusSending)).Exist(ctx)
}

// errNoEmail is the reason of a recipient whose Contact has no address to send to.
var errNoEmail = errors.New("contact has no email")

// fail ends a recipient's send as Failed with cause: Outbound send records the one message (a
// send that already went out stays sent). outboundmessages.ErrInProgress is a live claim: the caller
// waits and asks again.
func (d *Delivery) fail(ctx context.Context, s *ent.Scoped, rec *ent.BroadcastRecipient, cause error) error {
	res, err := d.out.FailRecipient(ctx, s, outboundmessages.Failure{Recipient: rec, Cause: cause, Stopped: cancelledAt(rec.BroadcastID)})
	if err != nil {
		return err
	}
	return d.settle(ctx, s, rec, res)
}

// settle takes what a decided Outbound outcome means for the Broadcast. The message already
// holds the recipient's outcome; a send that went through lifts the Broadcast's hold, if any.
func (d *Delivery) settle(ctx context.Context, s *ent.Scoped, rec *ent.BroadcastRecipient, res outboundmessages.Result) error {
	if res.Outcome != outboundmessages.Sent {
		return nil
	}
	return clearHold(ctx, s, rec.BroadcastID)
}

// retryLater is the recipient's turn when Outbound send wants a retry (a Hold or a
// Deferral): nothing was consumed, so the recipient stays pending.
func (d *Delivery) retryLater(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, rec *ent.BroadcastRecipient, res outboundmessages.Result) error {
	if res.Outcome == outboundmessages.Held {
		// Best effort: the hold is enforced by the send path and the caller retries on the
		// HeldError either way, so a failed write only leaves the reason unrecorded.
		if herr := setHold(ctx, s, b.ID, res.Hold); herr != nil {
			slog.WarnContext(ctx, "broadcast hold reason not recorded", "error", herr, "broadcast_id", b.ID)
		}
		return &HeldError{Reason: res.Hold}
	}
	// A send limit is spent, nothing is blocked: the Broadcast stays sending with no hold
	// reason and this recipient stays pending, retried after the wait. The wait is measured on
	// Outbound send's clock, so the time it is due is too.
	now := d.out.Now()
	ahead, cerr := s.BroadcastRecipient().Query().
		Where(
			broadcastrecipient.BroadcastID(rec.BroadcastID),
			Pending(),
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
	if uerr := s.BroadcastRecipient().UpdateOneID(rec.ID).SetDeferredUntil(now.Add(retry.After)).Exec(ctx); uerr != nil {
		slog.WarnContext(ctx, "broadcast recipient deferral not recorded", "error", uerr, "recipient_id", rec.ID)
	}
	return &DeferredError{Delay: retry.After, Cause: res.Cause}
}

// giveUp records the terminal failure of a recipient whose last attempt failed with cause:
// Outbound send ends its message as Failed with the cause as its reason (a send that already
// reached the provider stays sent), and the Broadcast settles if none is left pending. A
// recipient already decided keeps its outcome.
//
// outboundmessages.ErrInProgress means another attempt still holds a live claim and may yet reach the
// provider: nothing is failed, and the job waits out the lease and gives up again.
func (d *Delivery) giveUp(ctx context.Context, s *ent.Scoped, recipientID int64, cause error) error {
	rec, err := s.BroadcastRecipient().Get(ctx, recipientID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	b, err := s.Broadcast().Get(ctx, rec.BroadcastID)
	if err != nil {
		return fmt.Errorf("load broadcast %d: %w", rec.BroadcastID, err)
	}
	if stopped, err := cancelledBefore(ctx, s, b, rec); err != nil || stopped {
		return err
	}
	if err := d.fail(ctx, s, rec, cause); err != nil {
		return err
	}
	return finalize(ctx, s, rec.BroadcastID, d.out.Now())
}

// setHold records why a Broadcast is held: the reason Outbound send held it for, a
// Workspace-level one (holds.Reason*) or one of its source's own (outboundmessages.Hold*). The update
// refuses a reason the column does not declare. A cancelled Broadcast is held by nothing: a job
// that saw the hold before Cancel writes no reason after it. With clearHold it is the only writer
// of the hold reason.
func setHold(ctx context.Context, s *ent.Scoped, broadcastID int64, reason holdreason.Reason) error {
	_, err := s.Broadcast().Update().
		Where(broadcast.ID(broadcastID), broadcast.StatusNEQ(broadcast.StatusCancelled)).
		SetHoldReason(reason).
		Save(ctx)
	return err
}

// clearHold records that a Broadcast is no longer held.
func clearHold(ctx context.Context, s *ent.Scoped, broadcastID int64) error {
	return s.Broadcast().UpdateOneID(broadcastID).ClearHoldReason().Exec(ctx)
}
