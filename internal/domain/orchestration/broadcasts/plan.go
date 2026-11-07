package broadcasts

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/getsphericon/sphericon/ent"
	entbinding "github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/holdreason"
	"github.com/getsphericon/sphericon/internal/kernel/smsrender"
	"github.com/getsphericon/sphericon/internal/kernel/telegramrender"
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
func failPlan(ctx context.Context, s *ent.Scoped, id int64, cause error, at time.Time) error {
	if err := fail(ctx, s, id, at); err != nil {
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
// send time, so an unsubscribe or Suppression that lands after planning is honoured. The pace
// starts at Outbound send's clock, the one its Deferrals of the same recipients read.
func (d *Delivery) Run(ctx context.Context, s *ent.Scoped, id int64, sched Scheduler) error {
	return d.run(ctx, s, id, sched, triggerPlan)
}

// RunClaimed is Run for a job queued after the Broadcast was claimed (Send or Schedule): it
// refuses a draft with lifecycle.ErrInvalidTransition, so an unschedule that lands after
// Due has answered is not undone by the plan.
func (d *Delivery) RunClaimed(ctx context.Context, s *ent.Scoped, id int64, sched Scheduler) error {
	return d.run(ctx, s, id, sched, triggerResume)
}

func (d *Delivery) run(ctx context.Context, s *ent.Scoped, id int64, sched Scheduler, t trigger) error {
	ids, err := d.plan(ctx, s, id, t)
	if errors.Is(err, ErrPlanFailed) {
		return nil // the Broadcast failed once, recorded as such: a retry would only fail again
	}
	if err != nil || len(ids) == 0 {
		return err
	}
	times, err := d.Pace(ctx, s, id, ids, d.out.Now())
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
// was unscheduled after the job was queued. A delayed job
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
	content, err := outboundmessages.BroadcastContent(b)
	if err != nil {
		return nil, err
	}
	hold, err := d.out.Preflight(ctx, s, content, b.IntegrationID)
	if err != nil {
		return nil, fmt.Errorf("preflight broadcast %d: %w", b.ID, err)
	}
	if hold != "" {
		if err := setHold(ctx, s, b.ID, holdreason.Reason(hold)); err != nil {
			return nil, err
		}
		return nil, &HeldError{Reason: hold}
	}

	// The move wins the compare-and-swap before the hold is cleared, so a stale plan never
	// touches a Broadcast it lost.
	if err := begin(ctx, s, b.ID, t); err != nil {
		return nil, err
	}
	if err := clearHold(ctx, s, b.ID); err != nil {
		return nil, err
	}

	// Content that cannot render would fail every recipient: fail the broadcast once, here,
	// before any recipient row exists (ADR 0015).
	if verr := renderable(content); verr != nil {
		return nil, failPlan(ctx, s, b.ID, fmt.Errorf("broadcast %d: content does not render: %w", b.ID, verr), d.out.Now())
	}

	// The Broadcast is read again: begin has taken the copy of a draft's Condition. An audience
	// that cannot be resolved fails the Broadcast.
	armed, err := s.Broadcast().Get(ctx, b.ID)
	if err != nil {
		return nil, fmt.Errorf("load broadcast %d: %w", b.ID, err)
	}
	audience, err := audienceOf(ctx, s, armed, d.out.Now())
	if errors.Is(err, ErrNoAudience) {
		return nil, failPlan(ctx, s, b.ID, err, d.out.Now())
	}
	if err != nil {
		return nil, err
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
		if err := Finalize(ctx, s, b.ID, d.out.Now()); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// renderable is the Channel's own check that content renders at all, before anything is sent: on
// email a subject and body that Liquid and the HTML parser take.
func renderable(content outboundmessages.Content) error {
	switch c := content.(type) {
	case outboundmessages.EmailContent:
		return emailrender.Lint(c.Subject, c.Body, nil).Error
	case outboundmessages.TelegramContent:
		return telegramrender.Check(c.Text)
	case outboundmessages.SMSContent:
		return smsrender.Check(c.Text)
	default:
		return fmt.Errorf("broadcasts: no render check for %T", content)
	}
}

// ErrNoAudience marks a Broadcast whose audience cannot be resolved: a Segment Broadcast without
// the copy of its Condition, or a copy that no longer compiles. Its plan fails it.
var ErrNoAudience = errors.New("broadcasts: the audience of the broadcast cannot be resolved")

// destinationOf is the Destination the Broadcast b reaches its audience at, by its Channel (ADR
// 0080): on email the Contact's address, on SMS its phone, on Telegram its Bindings on the Integration the Broadcast
// routes through, the one it names or the Channel's default. A route with none reaches nobody.
func destinationOf(ctx context.Context, s *ent.Scoped, b *ent.Broadcast) (eligibility.Destination, error) {
	dest, err := destinationFor(ctx, s, b.Channel, b.IntegrationID)
	if err != nil {
		return nil, fmt.Errorf("broadcasts: broadcast %d: %w", b.ID, err)
	}
	return dest, nil
}

// destinationFor is destinationOf for a Channel and the Integration a Broadcast names (nil: the
// Channel's default), before a Broadcast exists.
func destinationFor(ctx context.Context, s *ent.Scoped, channel channels.Channel, integrationID *int64) (eligibility.Destination, error) {
	switch channel {
	case channels.Email:
		return eligibility.EmailDestination(), nil
	case channels.SMS:
		return eligibility.PhoneDestination(), nil
	case channels.Telegram:
		integ, err := messaging.Integration(ctx, s, channels.Telegram, integrationID)
		if errors.Is(err, messaging.ErrNoProvider) {
			return eligibility.BindingDestination(eligibility.ChannelTelegram, 0), nil
		}
		if err != nil {
			return nil, fmt.Errorf("integration: %w", err)
		}
		return eligibility.BindingDestination(eligibility.ChannelTelegram, integ.ID), nil
	default:
		return nil, fmt.Errorf("channel %q has no destination", channel)
	}
}

// RecipientsOf is how many of the Contacts that match the Condition cond (a Segment's rule, "" for
// all) a Broadcast on the Channel would take if it ran now, through the Integration it names (nil:
// the Channel's default): the audience of its plan, so the Contacts with an eligible Destination on
// that Channel. It is the audience the plan takes, asked before a Broadcast exists.
func RecipientsOf(ctx context.Context, s *ent.Scoped, channel channels.Channel, integrationID *int64, cond string, now time.Time) (int, error) {
	dest, err := destinationFor(ctx, s, channel, integrationID)
	if err != nil {
		return 0, err
	}
	pred, err := condition.ContactPredicate(ctx, s, cond, now)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrNoAudience, err)
	}
	return s.Contact().Query().Where(eligibility.Audience(dest, eligibility.SourceBroadcasts), pred).Count(ctx)
}

// audienceOf is the Contacts the plan of the Broadcast b, which has left draft, takes: the
// Workspace's Contacts narrowed by the Condition copied from its Segment when it left draft (ADR
// 0065; a Broadcast to all contacts is not narrowed) and by its Channel's Destination: they have
// one, and it is eligible. Eligibility is derived (ADR 0001): suppressed destinations and ones
// unsubscribed from the "broadcasts" source (or from everything) are excluded at the query level.
// A Contact without a Destination (on Telegram, an eligible Binding on the routed Integration) is left out, so no row is created that could never send and
// would block finalization forever.
func audienceOf(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, now time.Time) (*ent.ContactQuery, error) {
	dest, err := destinationOf(ctx, s, b)
	if err != nil {
		return nil, err
	}
	audience := s.Contact().Query().Where(eligibility.Audience(dest, eligibility.SourceBroadcasts))
	if b.SegmentID == nil {
		return audience, nil
	}
	if b.Condition == nil {
		return nil, fmt.Errorf("%w: broadcast %d: the condition of segment %d was not copied", ErrNoAudience, b.ID, *b.SegmentID)
	}
	pred, err := condition.ContactPredicate(ctx, s, *b.Condition, now)
	if err != nil {
		return nil, fmt.Errorf("%w: broadcast %d condition: %w", ErrNoAudience, b.ID, err)
	}
	return audience.Where(pred), nil
}

// AudienceSize is how many Contacts the plan of the Broadcast b, which has left draft, would take
// if it ran at now: an estimate for a scheduled Broadcast, whose audience is taken when it starts.
// ErrNoAudience when its plan would fail it instead.
func AudienceSize(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, now time.Time) (int, error) {
	audience, err := audienceOf(ctx, s, b, now)
	if err != nil {
		return 0, err
	}
	return audience.Count(ctx)
}

// spread is each send's time: a send starts as many steps after start as the calls to the provider
// the sends before it make, a step being one call's share of the limit.
func spread(start time.Time, step time.Duration, calls []int) []time.Time {
	times := make([]time.Time, len(calls))
	before := 0
	for i, n := range calls {
		times[i] = start.Add(time.Duration(before) * step)
		before += n
	}
	return times
}

// callsPerRecipient is how many calls to the provider each of the recipients makes, in their
// order: the Provider limit counts calls, not messages. On email it is one, on Telegram one per
// Binding the Contact has on the routed Integration, its own and its Visitors'. A Binding that
// turns out ineligible at send time still counts, so the estimate errs on the slow side. It is at
// least one: a recipient with no Binding left makes none, but is still paced.
func callsPerRecipient(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, route outboundmessages.Route, recipientIDs []int64) ([]int, error) {
	calls := make([]int, len(recipientIDs))
	for i := range calls {
		calls[i] = 1
	}
	if b.Channel != channels.Telegram || route.Integration == nil || len(recipientIDs) == 0 {
		return calls, nil
	}
	contacts, err := s.BroadcastRecipient().Query().
		Where(broadcastrecipient.IDIn(recipientIDs...)).
		Select(broadcastrecipient.FieldContactID).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("broadcasts: recipients of broadcast %d: %w", b.ID, err)
	}
	contactOf := make(map[int64]int64, len(contacts))
	owned := make([]int64, 0, len(contacts))
	for _, r := range contacts {
		contactOf[r.ID] = r.ContactID
		owned = append(owned, r.ContactID)
	}
	bound := make(map[int64]int, len(owned))
	for chunk := range slices.Chunk(owned, recipientInsertChunk) {
		rows, err := s.Binding().Query().
			Where(
				entbinding.IntegrationID(route.Integration.ID),
				entbinding.Or(entbinding.ContactIDIn(chunk...), entbinding.HasVisitorWith(visitor.ContactIDIn(chunk...))),
			).
			WithVisitor().
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("broadcasts: bindings of broadcast %d: %w", b.ID, err)
		}
		for _, row := range rows {
			switch {
			case row.ContactID != nil:
				bound[*row.ContactID]++
			case row.Edges.Visitor != nil && row.Edges.Visitor.ContactID != nil:
				bound[*row.Edges.Visitor.ContactID]++
			}
		}
	}
	for i, id := range recipientIDs {
		calls[i] = max(1, bound[contactOf[id]])
	}
	return calls, nil
}

// Pace spreads the sends to the recipients over time at the Provider limit of the Broadcast's Send
// route (ADR 0023): a send is scheduled one step after the one before it per call that one makes
// to the provider (a Telegram Contact with several chats makes one per chat), a step being one over
// the effective rate, so the limiter is a guard and a Deferral the rare case. The start is now, or
// the Broadcast's scheduled time when that is later. It records the last scheduled time on the
// Broadcast, from which the progress estimate is derived, and returns nil (every send at once)
// when nothing limits the route. A route that cannot send is not paced: the Broadcast is held
// (*HeldError, its reason recorded), as Send would hold each of its messages, so the pace and the
// hold never disagree. Pacing decides only when a send runs: eligibility is still checked per
// message at send time.
func (d *Delivery) Pace(ctx context.Context, s *ent.Scoped, id int64, recipientIDs []int64, now time.Time) ([]time.Time, error) {
	b, err := s.Broadcast().Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load broadcast %d: %w", id, err)
	}
	channel := b.Channel
	route, err := d.out.Route(ctx, s, channel, b.IntegrationID)
	if err != nil {
		return nil, err
	}
	if route.Held() {
		if err := setHold(ctx, s, b.ID, holdreason.Reason(route.Hold)); err != nil {
			return nil, err
		}
		return nil, &HeldError{Reason: route.Hold}
	}
	step := route.Interval()
	n := len(recipientIDs)
	if step <= 0 || n == 0 {
		return nil, s.Broadcast().UpdateOneID(b.ID).ClearLastScheduledAt().Exec(ctx)
	}
	start := now
	if b.ScheduledAt != nil && b.ScheduledAt.After(start) {
		start = *b.ScheduledAt
	}
	calls, err := callsPerRecipient(ctx, s, b, route, recipientIDs)
	if err != nil {
		return nil, err
	}
	times := spread(start, step, calls)
	return times, s.Broadcast().UpdateOneID(b.ID).SetLastScheduledAt(times[n-1]).Exec(ctx)
}
