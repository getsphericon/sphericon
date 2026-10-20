package broadcasts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/sendlimit"
)

// recipientInsertChunk bounds a single CreateBulk so a very large audience is planned in
// batches rather than one giant statement.
const recipientInsertChunk = 1000

// Send is one recipient's send to schedule. A zero At means as soon as possible: the
// Broadcast is not paced.
type Send struct {
	RecipientID int64
	At          time.Time
}

// Scheduler is the seam of the Broadcast lifecycle: "schedule this recipient's send at time
// T". The river adapter inserts a job per send at its time; the synchronous adapter sends in
// order at once. The module computes the times either way, so both run one sequence.
type Scheduler interface {
	Schedule(ctx context.Context, broadcastID int64, sends []Send) error
}

// ErrPlanFailed marks a plan that failed the Broadcast for good (a template that does not
// render, a broken segment): the Broadcast is failed, and running it again cannot help.
var ErrPlanFailed = errors.New("broadcasts: the plan failed the broadcast")

// failPlan moves the Broadcast to failed and reports cause as ErrPlanFailed. If the move
// itself fails the error carries no ErrPlanFailed, so the caller retries until it is recorded.
func failPlan(ctx context.Context, s *ent.Scoped, id int64, cause error) error {
	if err := fail(ctx, s, id); err != nil {
		return errors.Join(cause, err)
	}
	return fmt.Errorf("%w: %w", ErrPlanFailed, cause)
}

// Run is the whole delivery sequence of a Broadcast: plan the audience, pace the recipients
// and hand each send to the scheduler. It returns *HeldError when the Broadcast is held (it
// keeps its status and records why, to be run again), lifecycle.ErrInvalidTransition when it
// is past its plan (a stale job), and a plain error on a fault. A plan that failed the
// Broadcast (see ErrPlanFailed) is recorded and returns nil. An empty audience
// settles the Broadcast and schedules nothing. Eligibility is still checked per message at
// send time, so an unsubscribe or Suppression that lands after planning is honoured.
func (d *Delivery) Run(ctx context.Context, s *ent.Scoped, id int64, sched Scheduler, now time.Time) error {
	return d.run(ctx, s, id, sched, now, triggerPlan)
}

// RunClaimed is Run for a job queued after the Broadcast was claimed (Send or Schedule): it
// refuses a draft with lifecycle.ErrInvalidTransition, so an unschedule that lands after
// Due has answered is not undone by the plan.
func (d *Delivery) RunClaimed(ctx context.Context, s *ent.Scoped, id int64, sched Scheduler, now time.Time) error {
	return d.run(ctx, s, id, sched, now, triggerResume)
}

func (d *Delivery) run(ctx context.Context, s *ent.Scoped, id int64, sched Scheduler, now time.Time, t trigger) error {
	ids, err := d.plan(ctx, s, id, t)
	if errors.Is(err, ErrPlanFailed) {
		return nil // the Broadcast failed once, recorded as such: a retry would only fail again
	}
	if err != nil || len(ids) == 0 {
		return err
	}
	times, err := d.Pace(ctx, s, id, len(ids), now)
	if err != nil {
		return err
	}
	sends := make([]Send, len(ids))
	for i, rid := range ids {
		sends[i].RecipientID = rid
		if times != nil {
			sends[i].At = times[i]
		}
	}
	return sched.Schedule(ctx, id, sends)
}

// Due reports whether a queued send job should still run. A Broadcast that is back in draft
// was unscheduled (or its enqueue was reverted) after the job was queued. A delayed job
// (jobScheduledAt set) is also bound to the schedule it was enqueued for: unscheduling leaves
// it queued, so after a reschedule the Broadcast carries a different scheduled_at (or none,
// once sent now) and the superseded job must not send it early.
func Due(ctx context.Context, s *ent.Scoped, id int64, jobScheduledAt *time.Time) (bool, error) {
	b, err := s.Broadcast().Get(ctx, id)
	if err != nil {
		return false, fmt.Errorf("load broadcast %d: %w", id, err)
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

// Plan resolves the Broadcast's eligible audience, creates one pending recipient row per
// contact (idempotent through the unique (broadcast, contact) index, so a re-planned job
// converges) and returns the recipient ids to send. It refuses a Broadcast past its plan
// (lifecycle.ErrInvalidTransition) and holds one that cannot send now (*HeldError) before
// any row exists; a template that cannot render, or a broken segment, fails the Broadcast
// once, before any recipient row exists. An empty audience settles the Broadcast.
func (d *Delivery) Plan(ctx context.Context, s *ent.Scoped, id int64) ([]int64, error) {
	return d.plan(ctx, s, id, triggerPlan)
}

func (d *Delivery) plan(ctx context.Context, s *ent.Scoped, id int64, t trigger) ([]int64, error) {
	// A sent or failed Broadcast is past its plan: a late or retried job is refused here,
	// before a Hold reason is recorded on it.
	if err := canBegin(ctx, s, id, t); err != nil {
		return nil, err
	}
	b, err := s.Broadcast().Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load broadcast %d: %w", id, err)
	}

	// Ask Outbound send whether this workspace can send now, before any recipient row
	// exists: a Workspace freeze, no Integration, or an unverified From domain is a
	// reversible hold (ADR 0015): the broadcast keeps its status, records why, and the
	// plan is retried later; it is not failed.
	var from string
	if b.FromEmail != nil {
		from = *b.FromEmail
	}
	hold, err := d.out.Preflight(ctx, s, from)
	if err != nil {
		return nil, fmt.Errorf("preflight broadcast %d: %w", b.ID, err)
	}
	if hold != "" {
		if err := setHold(ctx, s, b.ID, hold); err != nil {
			return nil, err
		}
		return nil, &HeldError{Reason: hold}
	}

	// The move wins the compare-and-swap before the hold is cleared, so a stale plan never
	// touches a Broadcast it lost.
	if err := begin(ctx, s, b.ID, t); err != nil {
		return nil, err
	}
	if err := setHold(ctx, s, b.ID, ""); err != nil {
		return nil, err
	}

	// A template that cannot render would fail every recipient: fail the broadcast once,
	// here, before any recipient row exists (ADR 0015).
	if verr := emailrender.Lint(b.Subject, b.Body, nil).Error; verr != nil {
		return nil, failPlan(ctx, s, b.ID, fmt.Errorf("broadcast %d: template does not render: %w", b.ID, verr))
	}

	// Audience: contacts in the workspace with an email address, narrowed by the
	// broadcast's segment when set, then filtered to eligible recipients.
	// Eligibility is derived (ADR 0001): suppressed destinations and ones
	// unsubscribed from the "broadcasts" source (or from everything) are excluded
	// at the query level. EmailNotNil keeps un-sendable contacts out so no row is
	// created that could never send and would block finalization forever.
	audience := s.Contact().Query().Where(
		contact.EmailNotNil(),
		eligibility.Predicate(eligibility.ChannelEmail, eligibility.SourceBroadcasts),
	)
	if b.SegmentID != nil {
		seg, err := s.Segment().Query().Where(segment.IDEQ(*b.SegmentID)).Only(ctx)
		if err != nil {
			return nil, failPlan(ctx, s, b.ID, fmt.Errorf("load segment %d: %w", *b.SegmentID, err))
		}
		def := ""
		if seg.Definition != nil {
			def = *seg.Definition
		}
		pred, err := segments.ContactPredicate(ctx, s, def)
		if err != nil {
			return nil, failPlan(ctx, s, b.ID, fmt.Errorf("segment %d definition: %w", seg.ID, err))
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

	// Re-read the full row set so the returned ids are complete and retry-safe
	// (independent of which rows this attempt actually inserted).
	ids, err := s.BroadcastRecipient().Query().
		Where(broadcastrecipient.BroadcastID(b.ID)).
		IDs(ctx)
	if err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		if err := Finalize(ctx, s, b.ID); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// interval is the step between two sends of the Workspace's default Integration at its
// effective Send rate limit (ADR 0023): zero when nothing limits it (or there is no
// Integration). The planner and the progress estimate both pace by it.
func interval(ctx context.Context, s *ent.Scoped) (time.Duration, error) {
	integ, err := messaging.DefaultEmailIntegration(ctx, s)
	if errors.Is(err, messaging.ErrNoProvider) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return sendlimit.EffectiveOf(integ).Limits().Interval(), nil
}

// spread is the i-th send's time: i steps after start.
func spread(start time.Time, step time.Duration, n int) []time.Time {
	times := make([]time.Time, n)
	for i := range times {
		times[i] = start.Add(time.Duration(i) * step)
	}
	return times
}

// Pace spreads n recipient sends over time for a Broadcast whose Integration has a Send rate
// limit (ADR 0023): the i-th send is scheduled i steps after the start, a step being one over
// the effective rate, so the limiter is a guard and a Deferral the rare case. The start is
// now, or the Broadcast's scheduled time when that is later. It records the last scheduled
// time on the Broadcast, from which the progress estimate is derived, and returns nil (every
// send at once) when nothing limits the Integration. Pacing decides only when a send runs:
// eligibility is still checked per message at send time.
func (d *Delivery) Pace(ctx context.Context, s *ent.Scoped, id int64, n int, now time.Time) ([]time.Time, error) {
	b, err := s.Broadcast().Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load broadcast %d: %w", id, err)
	}
	step, err := interval(ctx, s)
	if err != nil {
		return nil, err
	}
	if step <= 0 || n == 0 {
		return nil, s.Broadcast().UpdateOneID(b.ID).ClearLastScheduledAt().Exec(ctx)
	}
	start := now
	if b.ScheduledAt != nil && b.ScheduledAt.After(start) {
		start = *b.ScheduledAt
	}
	times := spread(start, step, n)
	return times, s.Broadcast().UpdateOneID(b.ID).SetLastScheduledAt(times[n-1]).Exec(ctx)
}
