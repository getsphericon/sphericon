package jobs

import (
	"context"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/orders"
	"github.com/getsphericon/sphericon/internal/suppression"
)

// The consumers of the domain events that core owns (ADR 0101). Each is a job kind with its args
// type, which embeds events.EventJob, a worker, and an entry of EventConsumers, whose filter reads
// only the envelope. The application registers the entries on the bus when it is wired, and the
// Enterprise Edition registers its own beside them; the events module knows none of them.

// EventConsumers are core's consumers of the domain events, for the bus to insert their jobs.
func EventConsumers() []events.Consumer {
	return []events.Consumer{
		{Wants: suppresses, Job: func(env events.Envelope) river.JobArgs { return SuppressEventArgs{events.EventJob{Envelope: env}} }},
		{Wants: isOrder, Job: func(env events.Envelope) river.JobArgs { return OrderEventArgs{events.EventJob{Envelope: env}} }},
		{Wants: triggers, Job: func(env events.Envelope) river.JobArgs { return AutomationEventArgs{events.EventJob{Envelope: env}} }},
		{Wants: firesRules, Job: func(env events.Envelope) river.JobArgs { return RuleEventArgs{events.EventJob{Envelope: env}} }},
		{Wants: reachesWebhooks, Job: func(env events.Envelope) river.JobArgs { return WebhookEventArgs{events.EventJob{Envelope: env}} }},
	}
}

// WebhookEventArgs is the webhooks consumer's job (ADR 0039): an event to fan out to the
// Workspace's Webhook endpoints. The producer never reads the endpoints; the job does.
type WebhookEventArgs struct {
	events.EventJob
}

func (WebhookEventArgs) Kind() string { return "event_webhook" }

// reachesWebhooks wants every live event: loaded history is recorded, never delivered. Which
// endpoints receive one is the job's to find.
func reachesWebhooks(_ events.Envelope, ev events.DomainEvent) bool {
	return !events.IsBackfill(ev)
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

// Inserter queues a job. An event consumer that turns its event into work queues the work with
// it: the worker client in production, a client over the test's connection in a test, where the
// test reads what was queued.
type Inserter interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// triggers wants an Event a Contact's Automations may react to: one with a Contact, live (loaded
// history enrolls nobody) and made by a person (a scanner's open or click neither enrolls nor ends
// an Enrollment, ADR 0053). An administrative action (an Audit entry) is never one.
func triggers(_ events.Envelope, ev events.DomainEvent) bool {
	if _, unprojected := ev.(events.Unprojected); unprojected || events.IsBackfill(ev) {
		return false
	}
	p := ev.Project()
	return p.ContactID != 0 && !p.Machine
}

// firesRules wants what triggers wants, except an event a Rule's Action published (its envelope
// carries CausedBy): Rules never chain (ADR 0054).
func firesRules(env events.Envelope, ev events.DomainEvent) bool {
	return env.CausedBy == "" && triggers(env, ev)
}

// AutomationEventArgs is the automations consumer's job: an Event that may enroll its Contact, end
// an Enrollment of the Contact or bring one to its goal (#751).
type AutomationEventArgs struct {
	events.EventJob
}

func (AutomationEventArgs) Kind() string { return "event_automation" }

// AutomationEventWorker queues the evaluation of an Event against the Automations
// (EvaluateTriggerArgs), on the Event's semantic action, so a Trigger can be "contact.created" or a
// customer's "page_view". The Event's row was stored when it was published, so the evaluation finds
// it among the Contact's.
type AutomationEventWorker struct {
	river.WorkerDefaults[AutomationEventArgs]

	queue Inserter
}

// NewAutomationEventWorker is the automations consumer's worker, which queues on queue.
func NewAutomationEventWorker(queue Inserter) *AutomationEventWorker {
	return &AutomationEventWorker{queue: queue}
}

func (w *AutomationEventWorker) Work(ctx context.Context, job *river.Job[AutomationEventArgs]) error {
	env := job.Args.Envelope
	ev, err := events.Decode(env)
	if err != nil {
		return err
	}
	p := ev.Project()
	_, err = w.queue.Insert(ctx, EvaluateTriggerArgs{
		WorkspaceID:        env.WorkspaceID,
		ContactID:          p.ContactID,
		Action:             p.Action,
		SourceAutomationID: p.AutomationID,
		OccurredAt:         env.OccurredAt,
		SourceID:           env.SourceID(),
	}, nil)
	return err
}

// RuleEventArgs is the rules consumer's job (ADR 0054): an Event that may fire a Rule.
type RuleEventArgs struct {
	events.EventJob
}

func (RuleEventArgs) Kind() string { return "event_rule" }

// RuleEventWorker queues the evaluation of an Event against the Workspace's Rules
// (EvaluateRulesArgs). The Event's row was stored when it was published, so a Condition that counts
// the Contact's Events counts the one that fired the Rule. A Firing is unique on the Event's source
// id, so a retried job fires a Rule once.
type RuleEventWorker struct {
	river.WorkerDefaults[RuleEventArgs]

	queue Inserter
}

// NewRuleEventWorker is the rules consumer's worker, which queues on queue.
func NewRuleEventWorker(queue Inserter) *RuleEventWorker {
	return &RuleEventWorker{queue: queue}
}

func (w *RuleEventWorker) Work(ctx context.Context, job *river.Job[RuleEventArgs]) error {
	env := job.Args.Envelope
	ev, err := events.Decode(env)
	if err != nil {
		return err
	}
	p := ev.Project()
	_, err = w.queue.Insert(ctx, EvaluateRulesArgs{
		WorkspaceID: env.WorkspaceID,
		ContactID:   p.ContactID,
		Action:      p.Action,
		SourceID:    env.SourceID(),
		// Rules act only on events recorded after they were activated (ADR 0054).
		RecordedAt: env.RecordedAt,
		Properties: events.BoundedProperties(p.Properties),
	}, nil)
	return err
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
	p := ev.Project()
	kind, _ := p.Properties["bounceKind"].(string)
	_, ok := suppression.ForEvent(p.Action, kind)
	return ok
}

// SuppressEventWorker adds the address of a hard bounce or a complaint to the Workspace's
// Suppression list, once: a repeat keeps the first entry and its reason.
type SuppressEventWorker struct {
	river.WorkerDefaults[SuppressEventArgs]

	ent *ent.Client
}

// NewSuppressEventWorker is the suppression consumer's worker over the raw client: the envelope
// names the Workspace, which events.Suppress scopes to.
func NewSuppressEventWorker(client *ent.Client) *SuppressEventWorker {
	return &SuppressEventWorker{ent: client}
}

func (w *SuppressEventWorker) Work(ctx context.Context, job *river.Job[SuppressEventArgs]) error {
	return events.Suppress(ctx, w.ent, job.Args.Envelope)
}
