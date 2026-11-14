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

// SendRecipientArgs is one per-recipient send. BroadcastID rides along so the
// worker can finalize without an extra lookup.
type SendRecipientArgs struct {
	jobkind.Workspace

	RecipientID int64 `json:"recipient_id"`
	BroadcastID int64 `json:"broadcast_id"`
}

func (SendRecipientArgs) Kind() string { return jobkind.SendRecipient.Kind }

// InsertOpts puts a recipient's send in the broadcasts queue with broadcasts.RecipientAttempts.
// It is unique by args: a plan job retried after inserting some chunks plans the same recipients
// again, and river refuses a second job with the same args while the first is in any state but
// cancelled or discarded, so each recipient keeps one job.
func (SendRecipientArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       broadcasts.Queue,
		MaxAttempts: broadcasts.RecipientAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

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
	sched := riverSends{rc: river.ClientFromContext[*sql.Tx](ctx), workspaceID: s.WorkspaceID()}
	out := w.del.RunQueued(ctx, s, job.Args, sched)
	return riverResult(out)
}

// riverSends is the river adapter of the module's scheduling seam: one recipient job per send,
// scheduled at its time, named by the Broadcast's Workspace.
type riverSends struct {
	rc          *river.Client[*sql.Tx]
	workspaceID int64
}

func (r riverSends) Schedule(ctx context.Context, broadcastID int64, sends []broadcasts.Send) error {
	params := make([]river.InsertManyParams, len(sends))
	for i, send := range sends {
		// A zero At is no schedule: river runs the job at once.
		params[i] = river.InsertManyParams{
			Args:       SendRecipientArgs{WorkspaceID: r.workspaceID, RecipientID: send.RecipientID, BroadcastID: broadcastID},
			InsertOpts: &river.InsertOpts{ScheduledAt: send.At},
		}
	}
	return insertMany(ctx, r.rc, params)
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
	out := w.del.DeliverRecipient(ctx, s, job.Args.RecipientID, attempt)
	return riverResult(out)
}

// riverResult is the module's outcome as river reads it: a wait is a snooze, which spends no
// attempt, and any other error spends one.
func riverResult(out broadcasts.Outcome) error {
	if out.Wait > 0 {
		return river.JobSnooze(out.Wait)
	}
	return out.Err
}

// SendBroadcast runs a Broadcast's delivery sequence synchronously (broadcasts.Delivery.RunInline):
// the module plans and paces it exactly as production does and sends each recipient in order at
// once. It is the queue-free adapter for tests.
//
// Audience = workspace contacts with an email address (optionally narrowed by a
// segment) minus the ineligible (suppressed / unsubscribed, derived per ADR 0001).
func SendBroadcast(ctx context.Context, client *ent.Client, mod *outboundmessages.Module, broadcastID int64) error {
	s, err := broadcastScope(ctx, client, broadcastID)
	if err != nil {
		return err
	}
	return broadcasts.NewDelivery(mod).RunInline(ctx, s, broadcastID)
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
