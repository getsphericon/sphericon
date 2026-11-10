package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/domain/contacts/suppressions"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// The consumers of the domain events that core owns (ADR 0101). Each is an entry of EventConsumers,
// whose filter reads only the envelope, and the job the publish queues for it: a job kind of its
// own whose args embed events.EventJob, with a worker, or the evaluation the event asks for
// (against the Automations, against the Rules), queued at once. The application registers the entries on the bus when it is wired, and the
// Enterprise Edition registers its own beside them; the events module knows none of them.

// EventConsumers are core's consumers of the domain events, for the bus to insert their jobs.
func EventConsumers() []events.Consumer {
	return []events.Consumer{
		{Wants: observesMessage, Job: observeMessage, Replay: events.RetainForReplay[MessageObservationArgs](1)},
		{Wants: suppresses, Job: func(env events.Envelope, _ events.DomainEvent) river.JobArgs {
			return SuppressEventArgs{events.EventJob{Envelope: env}}
		}},
		{Wants: isOrder, Job: func(env events.Envelope, _ events.DomainEvent) river.JobArgs {
			return OrderEventArgs{events.EventJob{Envelope: env}}
		}},
		{Wants: triggers, Job: evaluateTrigger},
		{Wants: firesRules, Job: evaluateRules},
		{Wants: reachesWebhooks, Job: func(env events.Envelope, _ events.DomainEvent) river.JobArgs {
			return WebhookEventArgs{events.EventJob{Envelope: env}}
		}},
		{Wants: countsOnBroadcast, Job: func(env events.Envelope, ev events.DomainEvent) river.JobArgs {
			e, _ := ev.(*events.MessageUnsubscribed) // countsOnBroadcast established the type.
			return BroadcastUnsubscribeEventArgs{WorkspaceID: env.WorkspaceID, MessageID: e.OutboundMessageID, ObservedAt: e.ObservedAt}
		}, Replay: events.RetainForReplay[BroadcastUnsubscribeEventArgs](1)},
		{Wants: callsForNotices, Job: func(env events.Envelope, _ events.DomainEvent) river.JobArgs {
			return NoticeEventArgs{events.EventJob{Envelope: env}}
		}},
	}
}

// WebhookEventArgs is the webhooks consumer's job (ADR 0039): an event to fan out to the
// Workspace's Webhook endpoints. The producer never reads the endpoints; the job does.
type WebhookEventArgs struct {
	events.EventJob
}

func (WebhookEventArgs) Kind() string { return "event_webhook" }

// reachesWebhooks wants every live event a Workspace's endpoints may receive: an Event (loaded
// history is recorded, never delivered) or an Audit entry. An event of the Instance and a change
// told only to the Owners leave the platform through no endpoint. Which endpoints receive one is
// the job's to find.
func reachesWebhooks(_ events.Envelope, ev events.DomainEvent) bool {
	if _, audit := ev.(*events.AuditEntry); audit {
		return true
	}
	_, projected := ev.(events.Projected)
	return projected && !events.IsBackfill(ev)
}

// WebhookEventWorker serializes an event into its public webhook body (events.WebhookDeliveryOf)
// and hands it to the dispatcher, which queues one delivery job per enabled endpoint whose filter
// matches. It is a raw-client entry (ADR 0017): the envelope carries only a Workspace id, so the
// scope handed to the dispatcher is built here.
type WebhookEventWorker struct {
	river.WorkerDefaults[WebhookEventArgs]

	ent        *ent.Client
	dispatcher events.WebhookDispatcher
}

// NewWebhookEventWorker is the webhooks consumer's worker over the raw client and dispatcher.
func NewWebhookEventWorker(client *ent.Client, dispatcher events.WebhookDispatcher) *WebhookEventWorker {
	return &WebhookEventWorker{ent: client, dispatcher: dispatcher}
}

func (w *WebhookEventWorker) Work(ctx context.Context, job *river.Job[WebhookEventArgs]) error {
	env := job.Args.Envelope
	d, err := events.WebhookDeliveryOf(env)
	if err != nil {
		return err
	}
	return w.dispatcher.Dispatch(ctx, w.ent.Scoped(env.WorkspaceID), d.Name, d.ID, d.Body)
}

// triggers wants an Event a Contact's Automations may react to: one with a Contact, live (loaded
// history enrolls nobody) and made by a person (a scanner's open or click neither enrolls nor ends
// an Enrollment, ADR 0053). An administrative action (an Audit entry) is never one.
func triggers(_ events.Envelope, ev events.DomainEvent) bool {
	p, projected := events.ProjectionOf(ev)
	return projected && !events.IsBackfill(ev) && p.ContactID != 0 && !p.Machine
}

// firesRules wants what triggers wants, except an event a Rule's Action published (its envelope
// carries CausedBy): Rules never chain (ADR 0054).
func firesRules(env events.Envelope, ev events.DomainEvent) bool {
	return env.CausedBy == "" && triggers(env, ev)
}

// evaluateTrigger is the automations consumer's job: the evaluation of the Event against the
// Automations (EvaluateTriggerArgs), on its semantic action, so a Trigger can be "contact.created"
// or a customer's "page_view". The Event's row is stored by the publish that queues it, so the
// evaluation finds it among the Contact's.
func evaluateTrigger(env events.Envelope, ev events.DomainEvent) river.JobArgs {
	p, _ := events.ProjectionOf(ev)
	return EvaluateTriggerArgs{
		WorkspaceID:        env.WorkspaceID,
		ContactID:          p.ContactID,
		Action:             p.Action,
		SourceAutomationID: p.AutomationID,
		OccurredAt:         env.OccurredAt,
		SourceID:           env.SourceID(),
	}
}

// evaluateRules is the rules consumer's job (ADR 0054): the evaluation of the Event against the
// Workspace's Rules (EvaluateRulesArgs). The Event's row is stored by the publish that queues it, so
// a Condition that counts the Contact's Events counts the one that fired the Rule. A Firing is
// unique on the Event's source id, so a retried job fires a Rule once.
func evaluateRules(env events.Envelope, ev events.DomainEvent) river.JobArgs {
	p, _ := events.ProjectionOf(ev)
	return EvaluateRulesArgs{
		WorkspaceID: env.WorkspaceID,
		ContactID:   p.ContactID,
		Action:      p.Action,
		SourceID:    env.SourceID(),
		// Rules act only on events recorded after they were activated (ADR 0054).
		RecordedAt: env.RecordedAt,
		Properties: events.BoundedProperties(p.Properties),
	}
}

// OrderEventArgs is the orders consumer's job (ADR 0064): a collected order Event that makes or
// moves an Order.
type OrderEventArgs struct {
	events.EventJob
}

func (OrderEventArgs) Kind() string { return "event_order" }

// isOrder wants a collected order Event, loaded history included: a Backfill triggers no
// Automation and no Rule, but its Orders are revenue all the same.
func isOrder(_ events.Envelope, ev events.DomainEvent) bool {
	collected, ok := ev.(*events.CollectedEvent)
	return ok && orders.IsOrderAction(collected.Action)
}

// OrderEventWorker reads an order Event into the Workspace's Orders. Jobs run concurrently and in
// any order: events.RecordOrder takes a lock per Workspace, and an Event that arrives before the
// Order Completed it names is held until that one comes.
type OrderEventWorker struct {
	river.WorkerDefaults[OrderEventArgs]

	ent      *ent.Client
	recorder events.OrderRecorder
}

// NewOrderEventWorker is the orders consumer's worker over the raw client, which RecordOrder
// opens the Event's transaction on, and the Orders module.
func NewOrderEventWorker(client *ent.Client, recorder events.OrderRecorder) *OrderEventWorker {
	return &OrderEventWorker{ent: client, recorder: recorder}
}

func (w *OrderEventWorker) Work(ctx context.Context, job *river.Job[OrderEventArgs]) error {
	return events.RecordOrder(ctx, w.ent, w.recorder, job.Args.Envelope)
}

// SuppressEventArgs is the suppression consumer's job: a hard bounce or a complaint whose address
// the Workspace no longer sends to.
type SuppressEventArgs struct {
	events.EventJob
}

func (SuppressEventArgs) Kind() string { return "event_suppress" }

// suppresses wants an event that suppresses its address: a hard bounce or a complaint.
func suppresses(_ events.Envelope, ev events.DomainEvent) bool {
	p, _ := events.ProjectionOf(ev)
	kind, _ := p.Properties["bounceKind"].(string)
	_, ok := suppressions.ForEvent(p.Action, kind)
	return ok
}

// SuppressEventWorker adds the address of a hard bounce or a complaint to the Workspace's
// Suppression list, once: a repeat keeps the first entry and its reason.
type SuppressEventWorker struct {
	river.WorkerDefaults[SuppressEventArgs]

	ent *ent.Client
}

// NewSuppressEventWorker is the suppression consumer's worker over the raw client: the envelope
// names the Workspace, which Work scopes to.
func NewSuppressEventWorker(client *ent.Client) *SuppressEventWorker {
	return &SuppressEventWorker{ent: client}
}

// Work adds the event's destination to the Workspace's Suppression list when the action implies a
// global do-not-send (hard bounce, complaint). No-op for other actions or a missing email.
// Idempotent: a redelivered or repeated event keeps the original entry rather than erroring or
// overwriting its reason.
func (w *SuppressEventWorker) Work(ctx context.Context, job *river.Job[SuppressEventArgs]) error {
	env := job.Args.Envelope
	ev, err := events.Decode(env)
	if err != nil {
		return err
	}
	p, _ := events.ProjectionOf(ev)
	kind, _ := p.Properties["bounceKind"].(string)
	reason, ok := suppressions.ForEvent(p.Action, kind)
	if !ok {
		return nil
	}
	// The Workspace comes from the event envelope: a consumer has no membership or
	// api token, so this is a scope source outside the site/external/job list.
	s := w.ent.Scoped(env.WorkspaceID)
	// A refused destination ends the job (RefusalsEnd); any other error retries it.
	err = suppressions.Add(ctx, s, suppressions.Entry{Destination: p.Email, Reason: reason, ContactID: p.ContactID})
	if err != nil {
		return fmt.Errorf("suppress %q: %w", p.Email, err)
	}
	return nil
}

// BroadcastUnsubscribeEventArgs carries anonymous attribution to a received message on any
// Channel. It has no envelope or request metadata and survives Contact Erasure.
type BroadcastUnsubscribeEventArgs struct {
	jobkind.Workspace

	MessageID  int64     `json:"messageId"`
	ObservedAt time.Time `json:"observedAt"`
}

func (BroadcastUnsubscribeEventArgs) Kind() string { return "event_broadcast_unsubscribe" }
func (BroadcastUnsubscribeEventArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: events.QueueEvents}
}

// countsOnBroadcast selects the internal, anonymous attribution fact.
func countsOnBroadcast(_ events.Envelope, ev events.DomainEvent) bool {
	e, ok := ev.(*events.MessageUnsubscribed)
	return ok && e.OutboundMessageID != 0
}

// BroadcastUnsubscribeEventWorker updates the Broadcast owner through its anonymous message
// reference; neither a Contact lookup nor the original personal event is needed.
type BroadcastUnsubscribeEventWorker struct {
	river.WorkerDefaults[BroadcastUnsubscribeEventArgs]

	ent *ent.Client
}

// NewBroadcastUnsubscribeEventWorker is the broadcasts consumer's worker over the raw client: the
// envelope names the Workspace, which Work scopes to.
func NewBroadcastUnsubscribeEventWorker(client *ent.Client) *BroadcastUnsubscribeEventWorker {
	return &BroadcastUnsubscribeEventWorker{ent: client}
}

// Work marks the received message's recipient as unsubscribed, without restoring identity.
func (w *BroadcastUnsubscribeEventWorker) Work(ctx context.Context, job *river.Job[BroadcastUnsubscribeEventArgs]) error {
	if job.Args.MessageID == 0 {
		return nil
	}
	if _, err := broadcasts.RecordUnsubscribe(ctx, w.ent.Scoped(job.Args.WorkspaceID), job.Args.MessageID, job.Args.ObservedAt); err != nil {
		return fmt.Errorf("count the unsubscribe for the broadcast: %w", err)
	}
	return nil
}
