package broadcasts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// inProgressRetryDelay is how soon a job looks again at a send whose claim another attempt
// holds: it is not a failure and not a hold, just a race to wait out.
const inProgressRetryDelay = 30 * time.Second

// Attempt is the queue's count of a job's runs: Number is this one, from 1, and Limit the last.
// A runner with no retries runs every job once, as its last attempt: Attempt{Number: 1, Limit: 1}.
type Attempt struct {
	Number int
	Limit  int
}

func (a Attempt) last() bool { return a.Number >= a.Limit }

// Outcome is the queue-neutral result of one run of a send job (ADR 0111: the job runner holds no
// domain rule, it only turns the outcome into its own retry). The zero Outcome is a job that is
// done: it sent, there was nothing left to do, or the job was stale.
type Outcome struct {
	// Err is why the job is not done. With Wait it is the reason to wait; otherwise it spends an
	// attempt, and on the last one ends the job with it.
	Err error
	// Wait, when positive, asks again after it without spending an attempt: the source is held, a
	// send limit is spent or another attempt holds a live claim. Nothing was consumed.
	Wait time.Duration
	// GaveUp reports that the recipient's last attempt failed with Err and its failure is
	// recorded, the Broadcast settled: no further attempt can change it, so a runner moves on.
	GaveUp bool
}

// RunQueued runs a queued plan job: the delivery sequence of a Broadcast that was claimed for the
// job (Send or Schedule). Its plan may not claim a draft, so a Broadcast unscheduled since the job
// was queued stays one. A delayed job whose schedule was superseded, and a stale job, whose
// Broadcast is past its plan or back in draft, are done with nothing planned.
func (d *Delivery) RunQueued(ctx context.Context, s *ent.Scoped, args SendArgs, sched Scheduler) Outcome {
	if bound, err := boundToSchedule(ctx, s, args.BroadcastID, args.ScheduledAt); err != nil || !bound {
		return outcomeOf(err)
	}
	err := d.run(ctx, s, args.BroadcastID, sched, triggerResume)
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return Outcome{} // stale: unscheduled, sent or failed since the job was queued
	}
	return outcomeOf(err)
}

// boundToSchedule reports whether a queued plan job still answers the Broadcast's schedule. A job
// to send now (jobScheduledAt nil) always does. A delayed job is bound to the schedule it was
// enqueued for: unscheduling leaves it queued, so after a reschedule the Broadcast carries a
// different scheduled_at (or none, once sent now) and the superseded job must not send it early.
func boundToSchedule(ctx context.Context, s *ent.Scoped, id int64, jobScheduledAt *time.Time) (bool, error) {
	if jobScheduledAt == nil {
		return true, nil
	}
	b, err := s.Broadcast().Get(ctx, id)
	if err != nil {
		return false, fmt.Errorf("load broadcast %d: %w", id, err)
	}
	// Postgres keeps microseconds; the job payload keeps nanoseconds.
	return b.ScheduledAt != nil && b.ScheduledAt.Equal(jobScheduledAt.Truncate(time.Microsecond)), nil
}

// DeliverRecipient runs one attempt at a recipient's send job. A failed send spends the attempt,
// and on the last one the recipient is given up so the Broadcast can settle instead of hanging in
// sending. A recipient whose claim another attempt still holds is not given up: that attempt may
// yet reach the provider, so the job waits out the lease and gives up then.
func (d *Delivery) DeliverRecipient(ctx context.Context, s *ent.Scoped, recipientID int64, attempt Attempt) Outcome {
	err := d.SendRecipient(ctx, s, recipientID)
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return Outcome{} // decided meanwhile: a stale job
	}
	out := outcomeOf(err)
	if out.Err == nil || out.Wait > 0 || !attempt.last() {
		return out
	}
	gerr := d.giveUp(ctx, s, recipientID, err)
	if gerr != nil {
		// A give-up that cannot be recorded is returned with the failure, so the job is not lost.
		return outcomeOf(errors.Join(err, gerr))
	}
	return Outcome{Err: err, GaveUp: true}
}

// outcomeOf reads an error of the delivery: nil is done, a hold, a Deferral or a live claim waits
// the time it asks for, and anything else spends an attempt.
func outcomeOf(err error) Outcome {
	if _, held := errors.AsType[*HeldError](err); held {
		return Outcome{Err: err, Wait: outboundmessages.HoldRetry().After}
	}
	if deferred, ok := errors.AsType[*DeferredError](err); ok {
		return Outcome{Err: err, Wait: deferred.Delay}
	}
	if errors.Is(err, outboundmessages.ErrInProgress) {
		return Outcome{Err: err, Wait: inProgressRetryDelay}
	}
	return Outcome{Err: err}
}
