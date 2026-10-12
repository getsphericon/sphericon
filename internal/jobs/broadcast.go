package jobs

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/mokevnin/sphericon/ent"
	"github.com/mokevnin/sphericon/ent/broadcast"
	"github.com/mokevnin/sphericon/ent/broadcastrecipient"
	"github.com/mokevnin/sphericon/ent/contact"
	"github.com/mokevnin/sphericon/ent/outboundmessage"
	"github.com/mokevnin/sphericon/ent/segment"
	"github.com/mokevnin/sphericon/internal/broadcasts"
	"github.com/mokevnin/sphericon/internal/eligibility"
	"github.com/mokevnin/sphericon/internal/emailrender"
	"github.com/mokevnin/sphericon/internal/jobkind"
	"github.com/mokevnin/sphericon/internal/messaging"
	"github.com/mokevnin/sphericon/internal/outbound"
	"github.com/mokevnin/sphericon/internal/segments"
	"github.com/mokevnin/sphericon/internal/sendlimit"
)

// recipientInsertChunk bounds a single CreateBulk / InsertMany so a very large
// audience is planned in batches rather than one giant statement.
const recipientInsertChunk = 1000

// recipientMaxAttempts caps per-recipient send retries. A hard bounce won't
// recover, so the default 25 is wrong here; the failed recipient row records
// the last error.
const recipientMaxAttempts = 3

// SendBroadcastArgs is the fan-out (plan) job payload: which broadcast to
// dispatch. The worker resolves the audience and enqueues one per-recipient job
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

func (SendRecipientArgs) Kind() string { return jobkind.SendRecipient }

// SendBroadcastWorker is the fan-out phase: it plans the audience and enqueues a
// per-recipient job for each, so the send scales across workers instead of
// running the whole audience in one job under the default 1-minute JobTimeout.
type SendBroadcastWorker struct {
	river.WorkerDefaults[SendBroadcastArgs]
	ent *ent.Client
	mod *outbound.Module
}

// Timeout gives the plan phase room to resolve a large audience and enqueue the
// per-recipient jobs; the actual sending happens in those jobs, each bounded by
// the default per-job timeout.
func (w *SendBroadcastWorker) Timeout(*river.Job[SendBroadcastArgs]) time.Duration {
	return 10 * time.Minute
}

func (w *SendBroadcastWorker) Work(ctx context.Context, job *river.Job[SendBroadcastArgs]) error {
	if due, err := BroadcastDue(ctx, w.ent, job.Args.BroadcastID, job.Args.ScheduledAt); err != nil || !due {
		return err // not due: unscheduled (back to draft) after this job was queued
	}
	ids, err := PlanBroadcast(ctx, w.ent, w.mod, job.Args.BroadcastID)
	if err != nil {
		return snoozeIfDeferrable(err)
	}
	if len(ids) == 0 {
		return nil // empty audience: PlanBroadcast already finalized the broadcast
	}
	times, err := PaceRecipients(ctx, w.ent, job.Args.BroadcastID, len(ids), time.Now())
	if err != nil {
		return err
	}
	rc := river.ClientFromContext[pgx.Tx](ctx)
	for i := 0; i < len(ids); i += recipientInsertChunk {
		end := min(i+recipientInsertChunk, len(ids))
		params := make([]river.InsertManyParams, 0, end-i)
		for j, id := range ids[i:end] {
			opts := &river.InsertOpts{Queue: QueueBroadcasts, MaxAttempts: recipientMaxAttempts}
			if times != nil {
				opts.ScheduledAt = times[i+j]
			}
			params = append(params, river.InsertManyParams{
				Args:       SendRecipientArgs{RecipientID: id, BroadcastID: job.Args.BroadcastID},
				InsertOpts: opts,
			})
		}
		if _, err := rc.InsertMany(ctx, params); err != nil {
			return err
		}
	}
	return nil
}

// PaceRecipients spreads n recipient jobs over time for a Broadcast whose Integration
// has a Send rate limit (ADR 0023): the i-th job is scheduled i steps after the start,
// a step being one over the effective rate, so the limiter is a guard and a Deferral
// the rare case. The start is now, or the Broadcast's scheduled time when that is
// later. It records the last scheduled time on the Broadcast, from which the ETA is
// derived, and returns nil (plan as today, every job at once) when nothing limits the
// Integration. Pacing decides only when a job runs: eligibility is still checked per
// message at send time, so an unsubscribe or Suppression that lands meanwhile holds.
func PaceRecipients(ctx context.Context, client *ent.Client, broadcastID int64, n int, now time.Time) ([]time.Time, error) {
	b, err := client.Broadcast.Get(ctx, broadcastID)
	if err != nil {
		return nil, fmt.Errorf("load broadcast %d: %w", broadcastID, err)
	}
	s := client.Scoped(b.WorkspaceID)
	integ, err := messaging.DefaultEmailIntegration(ctx, s)
	if errors.Is(err, messaging.ErrNoProvider) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	step := sendlimit.EffectiveOf(integ).Limits().Interval()
	if step <= 0 || n == 0 {
		return nil, s.Broadcast().UpdateOneID(b.ID).ClearLastScheduledAt().Exec(ctx)
	}
	start := now
	if b.ScheduledAt != nil && b.ScheduledAt.After(start) {
		start = *b.ScheduledAt
	}
	times := make([]time.Time, n)
	for i := range times {
		times[i] = start.Add(time.Duration(i) * step)
	}
	return times, s.Broadcast().UpdateOneID(b.ID).SetLastScheduledAt(times[n-1]).Exec(ctx)
}

// BroadcastDue reports whether a queued send job should still run. A Broadcast that is
// back in draft was unscheduled (or its enqueue was reverted) after the job was queued.
// A delayed job (jobScheduledAt set) is also bound to the schedule it was enqueued for:
// unscheduling leaves it queued, so after a reschedule the Broadcast carries a different
// scheduled_at (or none, once sent now) and the superseded job must not send it early.
func BroadcastDue(ctx context.Context, client *ent.Client, broadcastID int64, jobScheduledAt *time.Time) (bool, error) {
	b, err := client.Broadcast.Get(ctx, broadcastID)
	if err != nil {
		return false, fmt.Errorf("load broadcast %d: %w", broadcastID, err)
	}
	if b.Status == broadcast.StatusDraft {
		return false, nil
	}
	if jobScheduledAt == nil {
		return true, nil
	}
	// Postgres keeps microseconds; the job payload keeps nanoseconds.
	return b.ScheduledAt != nil && b.ScheduledAt.Equal(jobScheduledAt.Truncate(time.Microsecond)), nil
}

// SendRecipientWorker delivers one broadcast recipient. Each recipient is its
// own job with its own retry budget, so a slow or failing send never blocks the
// rest of the audience.
type SendRecipientWorker struct {
	river.WorkerDefaults[SendRecipientArgs]
	ent *ent.Client
	mod *outbound.Module
}

func (w *SendRecipientWorker) Work(ctx context.Context, job *river.Job[SendRecipientArgs]) error {
	if err := SendToRecipient(ctx, w.ent, w.mod, job.Args.RecipientID); err != nil {
		if isDeferrable(err) {
			// A hold or a Deferral is not a failure: wait it out without spending an attempt.
			return snoozeIfDeferrable(err)
		}
		// On the final attempt, record the terminal failure so the broadcast can
		// finalize instead of hanging in "sending", then surface the error (river
		// discards the job and the ErrorHandler logs it).
		if job.Attempt >= job.MaxAttempts {
			_ = markRecipientFailed(ctx, w.ent, w.mod, job.Args.RecipientID, err)
			_ = FinalizeBroadcast(ctx, w.ent, job.Args.BroadcastID)
		}
		return err
	}
	// Cheap conditional no-op until the last recipient lands, at which point it
	// flips the broadcast to "sent" — no separate finalizer job needed.
	return FinalizeBroadcast(ctx, w.ent, job.Args.BroadcastID)
}

// SendBroadcast renders and sends a broadcast to its eligible audience
// synchronously (plan → send each → finalize). It is the pure, queue-free path:
// the river workers reuse the same PlanBroadcast/SendToRecipient/FinalizeBroadcast
// functions but fan the per-recipient sends out across jobs, while this composed
// form drives the whole send in one call for the inline adapter and tests.
//
// Audience = workspace contacts with an email address (optionally narrowed by a
// segment) minus the ineligible (suppressed / unsubscribed, derived per ADR 0001).
func SendBroadcast(ctx context.Context, client *ent.Client, mod *outbound.Module, broadcastID int64) error {
	ids, err := PlanBroadcast(ctx, client, mod, broadcastID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if serr := SendToRecipient(ctx, client, mod, id); serr != nil {
			if isDeferrable(serr) {
				// The source is on hold (or a claim is live): stop, leaving the remaining
				// recipients pending for a later run rather than consuming them.
				return serr
			}
			// Synchronous path has no retry runtime — a failed send is terminal, so
			// mark the row failed (a bad recipient must not abort the batch).
			_ = markRecipientFailed(ctx, client, mod, id, serr)
		}
	}
	return FinalizeBroadcast(ctx, client, broadcastID)
}

// PlanBroadcast resolves the broadcast's eligible audience, creates one pending
// BroadcastRecipient row per recipient (idempotent via the unique
// (broadcast, contact) index, so a re-planned job converges), and returns the
// recipient row IDs to send. It fails fast — before any rows exist — if the
// workspace has no usable sender, and finalizes immediately on an empty audience
// so the broadcast never hangs in "sending".
func PlanBroadcast(ctx context.Context, client *ent.Client, mod *outbound.Module, broadcastID int64) ([]int64, error) {
	b, err := client.Broadcast.Get(ctx, broadcastID)
	if err != nil {
		return nil, fmt.Errorf("load broadcast %d: %w", broadcastID, err)
	}
	// Job entry point: the scoped client is built from the loaded row's Workspace
	// (ADR 0017), then everything below goes through it.
	s := client.Scoped(b.WorkspaceID)

	// Ask Outbound send whether this workspace can send now, before any recipient row
	// exists: a Workspace freeze, no Integration, or an unverified From domain is a
	// reversible hold (ADR 0015) — the broadcast keeps its status, records why, and
	// the plan is retried later; it is not failed.
	var from string
	if b.FromEmail != nil {
		from = *b.FromEmail
	}
	hold, err := mod.Preflight(ctx, s, from)
	if err != nil {
		return nil, fmt.Errorf("preflight broadcast %d: %w", b.ID, err)
	}
	if hold != "" {
		setBroadcastHold(ctx, s, b.ID, hold)
		return nil, &HeldError{Reason: hold}
	}
	setBroadcastHold(ctx, s, b.ID, "")

	// A template that cannot render would fail every recipient: fail the broadcast once,
	// here, before any recipient row exists (ADR 0015).
	if verr := emailrender.Validate(b.Subject, b.Body); verr != nil {
		_ = s.Broadcast().UpdateOneID(b.ID).SetStatus(broadcast.StatusFailed).Exec(ctx)
		return nil, fmt.Errorf("broadcast %d: template does not render: %w", b.ID, verr)
	}

	if b, err = s.Broadcast().UpdateOneID(b.ID).SetStatus(broadcast.StatusSending).Save(ctx); err != nil {
		return nil, err
	}

	// Audience: contacts in the workspace with an email address, narrowed by the
	// broadcast's segment when set, then filtered to eligible recipients.
	// Eligibility is derived (ADR 0001) — suppressed destinations and ones
	// unsubscribed from the "broadcasts" source (or from everything) are excluded
	// at the query level. EmailNotNil keeps un-sendable contacts out so no row is
	// created that could never send and would block finalization forever.
	audience := s.Contact().Query().Where(
		contact.EmailNotNil(),
		eligibility.Predicate(eligibility.ChannelEmail, eligibility.SourceBroadcasts),
	)
	if b.SegmentID != nil {
		seg, err := s.Segment().Query().
			Where(segment.IDEQ(*b.SegmentID)).
			Only(ctx)
		if err != nil {
			_ = s.Broadcast().UpdateOneID(b.ID).SetStatus(broadcast.StatusFailed).Exec(ctx)
			return nil, fmt.Errorf("load segment %d: %w", *b.SegmentID, err)
		}
		def := ""
		if seg.Definition != nil {
			def = *seg.Definition
		}
		pred, err := segments.ContactPredicate(ctx, s, def)
		if err != nil {
			_ = s.Broadcast().UpdateOneID(b.ID).SetStatus(broadcast.StatusFailed).Exec(ctx)
			return nil, fmt.Errorf("segment %d definition: %w", seg.ID, err)
		}
		audience = audience.Where(pred)
	}

	contactIDs, err := audience.IDs(ctx)
	if err != nil {
		return nil, err
	}

	// Create one pending row per recipient, batched. OnConflict-ignore makes a
	// re-planned (retried) job converge instead of tripping the unique index.
	for i := 0; i < len(contactIDs); i += recipientInsertChunk {
		end := min(i+recipientInsertChunk, len(contactIDs))
		builders := make([]*ent.BroadcastRecipientScopedCreate, 0, end-i)
		for _, cid := range contactIDs[i:end] {
			builders = append(builders, s.BroadcastRecipient().Create().
				SetBroadcastID(b.ID).
				SetContactID(cid))
		}
		if err := s.BroadcastRecipient().CreateBulk(builders...).
			OnConflictColumns("broadcast_id", "contact_id").
			Ignore().
			Exec(ctx); err != nil {
			return nil, err
		}
	}

	// Re-read the full row set so the returned IDs are complete and retry-safe
	// (independent of which rows this attempt actually inserted).
	ids, err := s.BroadcastRecipient().Query().
		Where(broadcastrecipient.BroadcastID(b.ID)).
		IDs(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.Broadcast().UpdateOneID(b.ID).SetRecipientsTotal(len(ids)).Exec(ctx); err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		if err := FinalizeBroadcast(ctx, client, b.ID); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// SendToRecipient hands one broadcast recipient's email to Outbound send and maps
// the Outcome onto the recipient row. It is idempotent: a recipient already at a
// final status is left alone, and the module's idempotency key makes a retry after
// a committed send replay the recorded result. A returned error is retryable
// (provider down, claim in flight); a *HeldError means the source is on hold and a
// *DeferredError that its Send rate limit is spent; either way the job should be
// deferred, not failed.
func SendToRecipient(ctx context.Context, client *ent.Client, mod *outbound.Module, recipientID int64) error {
	rec, err := client.BroadcastRecipient.Get(ctx, recipientID)
	if ent.IsNotFound(err) {
		return nil // erased with its Contact (ADR 0021) after the job was queued: nothing to send
	}
	if err != nil {
		return fmt.Errorf("load recipient %d: %w", recipientID, err)
	}
	// Job entry point: the scoped client is built from the loaded row's Workspace.
	s := client.Scoped(rec.WorkspaceID)
	if rec.Status != broadcastrecipient.StatusPending {
		return nil // already decided (retry after a committed send) — don't re-send
	}

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
	// between plan and send. Terminal — the row can't ever send.
	if c.Email == nil {
		_ = s.BroadcastRecipient().UpdateOneID(rec.ID).SetStatus(broadcastrecipient.StatusFailed).SetError("contact has no email").Exec(ctx)
		return nil
	}

	req := outbound.Request{
		Kind:        outboundmessage.KindBroadcast,
		Key:         recipientKey(rec.ID),
		Destination: *c.Email,
		Contact:     c,
		Source:      eligibility.SourceBroadcasts,
		Subject:     b.Subject,
		Body:        b.Body,
		TrackID:     rec.ID,
		Ref:         outbound.Ref{BroadcastID: b.ID, BroadcastRecipient: rec.ID},
	}
	if b.FromEmail != nil {
		req.FromEmail = *b.FromEmail
	}
	if b.FromName != nil {
		req.FromName = *b.FromName
	}

	res, err := mod.Send(ctx, s, req)
	if err != nil {
		return err
	}
	upd := s.BroadcastRecipient().UpdateOneID(rec.ID)
	if res.MessageID != 0 {
		upd.SetOutboundMessageID(res.MessageID)
	}
	switch res.Outcome {
	case outbound.Sent:
		setBroadcastHold(ctx, s, b.ID, "") // a send went through: the hold, if any, has lifted
		err = upd.SetStatus(broadcastrecipient.StatusSent).SetSentAt(time.Now()).Exec(ctx)
	case outbound.Skipped:
		err = upd.SetStatus(broadcastrecipient.StatusSkipped).SetError(res.Reason).Exec(ctx)
	case outbound.Failed:
		err = upd.SetStatus(broadcastrecipient.StatusFailed).SetError(res.Reason).Exec(ctx)
	case outbound.Deferral:
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
		delay := deferralDelay(res.Wait, ahead, rand.Float64())
		// Best effort, like the count: it only sharpens later jobs' backlog.
		_ = upd.SetDeferredUntil(now.Add(delay)).Exec(ctx)
		return &DeferredError{Wait: res.Wait, Backlog: ahead, Delay: delay}
	default: // outbound.Held
		setBroadcastHold(ctx, s, b.ID, res.Reason)
		return &HeldError{Reason: res.Reason}
	}
	return err
}

// recipientKey is the Outbound send idempotency key of one broadcast recipient.
func recipientKey(recipientID int64) string {
	return fmt.Sprintf("broadcast:%d", recipientID)
}

// markRecipientFailed records a terminal delivery failure on a recipient row and
// gives up the Outbound claim so a stale pending message does not linger.
func markRecipientFailed(ctx context.Context, client *ent.Client, mod *outbound.Module, recipientID int64, cause error) error {
	rec, err := client.BroadcastRecipient.Get(ctx, recipientID)
	if err != nil {
		return err
	}
	s := client.Scoped(rec.WorkspaceID)
	_ = mod.MarkFailed(ctx, s, recipientKey(recipientID), cause)
	return s.BroadcastRecipient().UpdateOneID(rec.ID).
		SetStatus(broadcastrecipient.StatusFailed).
		SetError(cause.Error()).
		Exec(ctx)
}

// FinalizeBroadcast loads the broadcast and settles it once none of its recipients
// are pending (see broadcasts.Finalize).
func FinalizeBroadcast(ctx context.Context, client *ent.Client, broadcastID int64) error {
	b, err := client.Broadcast.Get(ctx, broadcastID)
	if ent.IsNotFound(err) {
		return nil // the broadcast was deleted: nothing to finalize
	}
	if err != nil {
		return fmt.Errorf("load broadcast %d: %w", broadcastID, err)
	}
	// Job entry point: the scoped client is built from the loaded row's Workspace.
	s := client.Scoped(b.WorkspaceID)

	return broadcasts.Finalize(ctx, s, broadcastID)
}
