package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// recipientInsertChunk bounds a single InsertMany so a very large audience is enqueued in
// batches rather than one giant statement.
const recipientInsertChunk = 1000

// recipientMaxAttempts caps per-recipient send retries. A hard bounce won't
// recover, so the default 25 is wrong here; the failed recipient row records
// the last error.
const recipientMaxAttempts = 3

// SendRecipientArgs is one per-recipient send. BroadcastID rides along so the
// worker can finalize without an extra lookup.
type SendRecipientArgs struct {
	jobkind.Workspace

	RecipientID int64 `json:"recipient_id"`
	BroadcastID int64 `json:"broadcast_id"`
}

func (SendRecipientArgs) Kind() string { return jobkind.SendRecipient.Kind }

// SendBroadcastWorker is the fan-out phase: it runs the module's delivery sequence with a
// river scheduler, so the send scales across workers instead of running the whole audience
// in one job under the default 1-minute JobTimeout.
type SendBroadcastWorker struct {
	river.WorkerDefaults[broadcasts.SendArgs]

	ent *ent.Client
	del *broadcasts.Delivery
}

// NewSendBroadcastWorker builds the worker that plans a Broadcast's audience and sends through mod.
func NewSendBroadcastWorker(client *ent.Client, mod *outboundmessages.Module) *SendBroadcastWorker {
	return &SendBroadcastWorker{ent: client, del: broadcasts.NewDelivery(mod)}
}

// Timeout gives the plan phase room to resolve a large audience and enqueue the
// per-recipient jobs; the actual sending happens in those jobs, each bounded by
// the default per-job timeout.
func (w *SendBroadcastWorker) Timeout(*river.Job[broadcasts.SendArgs]) time.Duration {
	return 10 * time.Minute
}

// Work is the river adapter over the module's plan job: it keeps only the queue concerns, the
// schedule of the recipient jobs and the snooze or retry the module's outcome asks for.
func (w *SendBroadcastWorker) Work(ctx context.Context, job *river.Job[broadcasts.SendArgs]) error {
	s, err := broadcastScope(ctx, w.ent, job.Args.BroadcastID)
	if err != nil {
		return err
	}
	return riverResult(w.del.RunQueued(ctx, s, job.Args, riverSends{rc: river.ClientFromContext[*sql.Tx](ctx), workspaceID: s.WorkspaceID()}))
}

// riverSends is the river adapter of the module's scheduling seam: one recipient job per send,
// scheduled at its time, named by the Broadcast's Workspace.
type riverSends struct {
	rc          *river.Client[*sql.Tx]
	workspaceID int64
}

func (r riverSends) Schedule(ctx context.Context, broadcastID int64, sends []broadcasts.Send) error {
	for i := 0; i < len(sends); i += recipientInsertChunk {
		chunk := sends[i:min(i+recipientInsertChunk, len(sends))]
		params := make([]river.InsertManyParams, 0, len(chunk))
		for _, send := range chunk {
			// A plan job retried after inserting some chunks plans the same recipients again:
			// river refuses a second job with the same args while the first is in any state
			// but cancelled or discarded, so each recipient keeps one job.
			opts := &river.InsertOpts{Queue: broadcasts.Queue, MaxAttempts: recipientMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true}}
			if !send.At.IsZero() {
				opts.ScheduledAt = send.At
			}
			params = append(params, river.InsertManyParams{
				Args:       SendRecipientArgs{WorkspaceID: r.workspaceID, RecipientID: send.RecipientID, BroadcastID: broadcastID},
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

// NewSendRecipientWorker builds the worker that sends one Broadcast recipient through mod.
func NewSendRecipientWorker(client *ent.Client, mod *outboundmessages.Module) *SendRecipientWorker {
	return &SendRecipientWorker{ent: client, del: broadcasts.NewDelivery(mod)}
}

// Work is the river adapter over the module's recipient job: it hands the module the attempt and
// turns the outcome into river's snooze or retry. On the last attempt a given-up recipient's
// failure is still returned, so river discards the job and the ErrorHandler logs it.
func (w *SendRecipientWorker) Work(ctx context.Context, job *river.Job[SendRecipientArgs]) error {
	s, err := recipientScope(ctx, w.ent, job.Args.RecipientID)
	if err != nil || s == nil {
		return err // no such recipient: erased with its Contact after the job was queued
	}
	attempt := broadcasts.Attempt{Number: job.Attempt, Limit: job.MaxAttempts}
	return riverResult(w.del.DeliverRecipient(ctx, s, job.Args.RecipientID, attempt))
}

// riverResult is the module's outcome as river reads it: a wait is a snooze, which spends no
// attempt, and any other error spends one.
func riverResult(out broadcasts.Outcome) error {
	if out.Wait > 0 {
		return river.JobSnooze(out.Wait)
	}
	return out.Err
}

// SendBroadcast runs a Broadcast's delivery sequence synchronously: the module plans and
// paces it exactly as production does, and the inline scheduler sends each recipient in
// order at once. It is the queue-free adapter for the inline enqueuer and tests.
//
// Audience = workspace contacts with an email address (optionally narrowed by a
// segment) minus the ineligible (suppressed / unsubscribed, derived per ADR 0001).
func SendBroadcast(ctx context.Context, client *ent.Client, mod *outboundmessages.Module, broadcastID int64) error {
	s, err := broadcastScope(ctx, client, broadcastID)
	if err != nil {
		return err
	}
	del := broadcasts.NewDelivery(mod)
	return del.Run(ctx, s, broadcastID, inlineSends{s: s, del: del})
}

// inlineSends is the synchronous adapter of the module's scheduling seam: it sends in order,
// ignoring the times (the module still computes and records them).
type inlineSends struct {
	s   *ent.Scoped
	del *broadcasts.Delivery
}

func (i inlineSends) Schedule(ctx context.Context, _ int64, sends []broadcasts.Send) error {
	for _, send := range sends {
		// No retry runtime: every send's first attempt is its last, so a failed one is given up
		// at once (a bad recipient must not abort the batch).
		out := i.del.DeliverRecipient(ctx, i.s, send.RecipientID, broadcasts.Attempt{Number: 1, Limit: 1})
		if out.Err != nil && !out.GaveUp {
			// A hold, a Deferral or a live claim stops the run, leaving the remaining recipients
			// pending for a later one; so does a give-up that could not be recorded.
			return out.Err
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
