package jobs

import (
	"context"

	"github.com/riverqueue/river"

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
