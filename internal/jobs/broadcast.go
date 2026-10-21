package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/outbound"
)

// recipientInsertChunk bounds a single InsertMany so a very large audience is enqueued in
// batches rather than one giant statement.
const recipientInsertChunk = 1000

// recipientMaxAttempts caps per-recipient send retries. A hard bounce won't
// recover, so the default 25 is wrong here; the failed recipient row records
// the last error.
const recipientMaxAttempts = 3

// SendBroadcastArgs is the fan-out (plan) job payload: which broadcast to
// dispatch. The worker plans the audience and enqueues one per-recipient job
// each, so no single job carries the whole send.
type SendBroadcastArgs struct {
	BroadcastID int64 `json:"broadcast_id"`
	// ScheduledAt is the time a delayed job was enqueued for; nil for an immediate send.
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
}

func (SendBroadcastArgs) Kind() string { return "send_broadcast" }

// SendRecipientArgs is one per-recipient send. BroadcastID rides along so the
// worker can finalize without an extra lookup.
type SendRecipientArgs struct {
	RecipientID int64 `json:"recipient_id"`
	BroadcastID int64 `json:"broadcast_id"`
}

func (SendRecipientArgs) Kind() string { return jobkind.SendRecipient.Kind }

// SendBroadcastWorker is the fan-out phase: it runs the module's delivery sequence with a
// river scheduler, so the send scales across workers instead of running the whole audience
// in one job under the default 1-minute JobTimeout.
type SendBroadcastWorker struct {
	river.WorkerDefaults[SendBroadcastArgs]
	ent *ent.Client
	del *broadcasts.Delivery
}

// Timeout gives the plan phase room to resolve a large audience and enqueue the
// per-recipient jobs; the actual sending happens in those jobs, each bounded by
// the default per-job timeout.
func (w *SendBroadcastWorker) Timeout(*river.Job[SendBroadcastArgs]) time.Duration {
	return 10 * time.Minute
}

// Work is the river adapter over the module's delivery sequence: it keeps only the queue
// concerns, the schedule of the recipient jobs and the retry decision of a hold.
func (w *SendBroadcastWorker) Work(ctx context.Context, job *river.Job[SendBroadcastArgs]) error {
	s, err := broadcastScope(ctx, w.ent, job.Args.BroadcastID)
	if err != nil {
		return err
	}
	if due, err := broadcasts.Due(ctx, s, job.Args.BroadcastID, job.Args.ScheduledAt); err != nil || !due {
		return err // not due: unscheduled (back to draft) after this job was queued
	}
	// RunClaimed refuses a Broadcast unscheduled after the check above: Due is the shortcut
	// of the usual case, the claim inside the plan decides the race.
	err = w.del.RunClaimed(ctx, s, job.Args.BroadcastID, riverSends{river.ClientFromContext[*sql.Tx](ctx)}, time.Now())
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return nil // stale: the Broadcast is past its plan (sent or failed), nothing to retry
	}
	return snoozeIfDeferrable(err)
}

// riverSends is the river adapter of the module's scheduling seam: one recipient job per send,
// scheduled at its time.
type riverSends struct{ rc *river.Client[*sql.Tx] }

func (r riverSends) Schedule(ctx context.Context, broadcastID int64, sends []broadcasts.Send) error {
	for i := 0; i < len(sends); i += recipientInsertChunk {
		chunk := sends[i:min(i+recipientInsertChunk, len(sends))]
		params := make([]river.InsertManyParams, 0, len(chunk))
		for _, send := range chunk {
			opts := &river.InsertOpts{Queue: QueueBroadcasts, MaxAttempts: recipientMaxAttempts}
			if !send.At.IsZero() {
				opts.ScheduledAt = send.At
			}
			params = append(params, river.InsertManyParams{
				Args:       SendRecipientArgs{RecipientID: send.RecipientID, BroadcastID: broadcastID},
				InsertOpts: opts,
			})
		}
		if _, err := r.rc.InsertMany(ctx, params); err != nil {
			return err
		}
	}
	return nil
}

// SendRecipientWorker delivers one broadcast recipient. Each recipient is its
// own job with its own retry budget, so a slow or failing send never blocks the
// rest of the audience.
type SendRecipientWorker struct {
	river.WorkerDefaults[SendRecipientArgs]
	ent *ent.Client
	del *broadcasts.Delivery
}

// Work is the river adapter over the module's recipient delivery: it keeps only the queue
// concerns, snoozing on a retry decision, the last attempt and finishing a stale job.
func (w *SendRecipientWorker) Work(ctx context.Context, job *river.Job[SendRecipientArgs]) error {
	s, err := recipientScope(ctx, w.ent, job.Args.RecipientID)
	if err != nil || s == nil {
		return err // no such recipient: erased with its Contact after the job was queued
	}
	err = w.del.SendRecipient(ctx, s, job.Args.RecipientID)
	switch classify(err) {
	case outcomeDone:
		return nil
	case outcomeRetryLater:
		// A hold or a Deferral is not a failure: wait it out without spending an attempt.
		return snoozeIfDeferrable(err)
	case outcomeFailed:
		// falls through to the retry budget below
	}
	// On the final attempt the module records the terminal failure so the Broadcast can
	// finalize instead of hanging in "sending"; the error still surfaces (river discards
	// the job and the ErrorHandler logs it). A give-up that cannot be recorded is returned
	// with it, so the job is not silently lost.
	if job.Attempt >= job.MaxAttempts {
		if gerr := w.del.GiveUp(ctx, s, job.Args.RecipientID, err); gerr != nil {
			if errors.Is(gerr, outbound.ErrInProgress) {
				// Another attempt holds a live claim and may still reach the provider, so
				// nothing is failed yet. A snooze spends no attempt: the job runs again once
				// the lease is out, and gives up (or replays the recorded send) then.
				return river.JobSnooze(inProgressRetryDelay)
			}
			return errors.Join(err, gerr)
		}
	}
	return err
}

// outcome is how an adapter treats the result of delivering one recipient.
type outcome int

const (
	outcomeDone       outcome = iota // delivered, or nothing left to do (a stale job included)
	outcomeRetryLater                // a hold, a Deferral or a live claim: not a failure
	outcomeFailed                    // the attempt failed
)

// classify is the one reading of a delivery error shared by both adapters. A recipient that
// was decided meanwhile (lifecycle.ErrInvalidTransition) is a stale job, finished without a
// retry.
func classify(err error) outcome {
	switch {
	case err == nil, errors.Is(err, lifecycle.ErrInvalidTransition):
		return outcomeDone
	case broadcasts.IsRetryLater(err):
		return outcomeRetryLater
	default:
		return outcomeFailed
	}
}

// SendBroadcast runs a Broadcast's delivery sequence synchronously: the module plans and
// paces it exactly as production does, and the inline scheduler sends each recipient in
// order at once. It is the queue-free adapter for the inline enqueuer and tests.
//
// Audience = workspace contacts with an email address (optionally narrowed by a
// segment) minus the ineligible (suppressed / unsubscribed, derived per ADR 0001).
func SendBroadcast(ctx context.Context, client *ent.Client, mod *outbound.Module, broadcastID int64) error {
	s, err := broadcastScope(ctx, client, broadcastID)
	if err != nil {
		return err
	}
	del := broadcasts.NewDelivery(mod)
	return del.Run(ctx, s, broadcastID, inlineSends{s: s, del: del}, time.Now())
}

// inlineSends is the synchronous adapter of the module's scheduling seam: it sends in order,
// ignoring the times (the module still computes and records them).
type inlineSends struct {
	s   *ent.Scoped
	del *broadcasts.Delivery
}

func (i inlineSends) Schedule(ctx context.Context, _ int64, sends []broadcasts.Send) error {
	for _, send := range sends {
		serr := i.del.SendRecipient(ctx, i.s, send.RecipientID)
		switch classify(serr) {
		case outcomeDone:
			// delivered, or stale: next recipient
		case outcomeRetryLater:
			// The source is on hold (or a claim is live): stop, leaving the remaining
			// recipients pending for a later run rather than consuming them.
			return serr
		case outcomeFailed:
			// Unlike the river adapter, which retries up to the last attempt, the synchronous
			// path has no retry runtime: a failed send is terminal, so give the recipient up
			// at once (a bad recipient must not abort the batch).
			if gerr := i.del.GiveUp(ctx, i.s, send.RecipientID, serr); gerr != nil {
				if errors.Is(gerr, outbound.ErrInProgress) {
					return gerr // a live claim: stop and leave the rest pending, like a hold
				}
				return errors.Join(serr, gerr)
			}
		}
	}
	return nil
}

// broadcastScope loads the Broadcast row that names the Workspace and scopes to it (the job
// entry point, ADR 0017).
func broadcastScope(ctx context.Context, client *ent.Client, broadcastID int64) (*ent.Scoped, error) {
	b, err := client.Broadcast.Get(ctx, broadcastID)
	if err != nil {
		return nil, fmt.Errorf("load broadcast %d: %w", broadcastID, err)
	}
	return client.Scoped(b.WorkspaceID), nil
}

// recipientScope loads the recipient row that names the Workspace and scopes to it (the job
// entry point, ADR 0017). It returns nil, nil for a recipient that no longer exists.
func recipientScope(ctx context.Context, client *ent.Client, recipientID int64) (*ent.Scoped, error) {
	rec, err := client.BroadcastRecipient.Get(ctx, recipientID)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load recipient %d: %w", recipientID, err)
	}
	return client.Scoped(rec.WorkspaceID), nil
}
