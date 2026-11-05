package events

import (
	"context"

	"github.com/riverqueue/river"
)

// A domain event reaches its consumers as River jobs (ADR 0101): Publish inserts, in the
// producer's transaction, one job for every consumer that wants the event, so a job exists iff
// the state change that published its event commits. River retries a failing job on its own,
// without holding up the other consumers or the other events, keeps it as discarded when its
// attempts run out, and an operator retries it from River UI. There is no ordering between the
// jobs: a consumer that needs one takes a lock of its own.

// QueueEvents is the queue every event consumer's job runs in.
const QueueEvents = "events"

// Consumer is one consumer of the domain events: which events it wants and the job it is handed
// for each. Producers never learn about it: the consumers are registered on the Bus when the
// application is wired (UseConsumers), core's and the Enterprise Edition's alike.
type Consumer struct {
	// Wants reports whether the consumer handles the event. It reads only the envelope and the
	// event decoded from it, never the database, so deciding who gets an event costs no query.
	Wants func(env Envelope, ev DomainEvent) bool
	// Job is the args of the consumer's job for env. River tells a job's kind by the Go type of
	// its args, so each consumer has an args type of its own, which embeds EventJob.
	Job func(env Envelope) river.JobArgs
}

// EventJob is what the args of every event consumer's job embed: the whole envelope, so the
// consumer reads the typed event, its version, its times and its chain, and runs in the events
// queue. The envelope's fields are the args' own, so every event job names a Workspace, a
// Contact or an address at the same place, where Erasure finds them (PurgeEventJobs).
type EventJob struct {
	Envelope
}

// InsertOpts puts every event job in QueueEvents.
func (EventJob) InsertOpts() river.InsertOpts { return river.InsertOpts{Queue: QueueEvents} }

// UseConsumers registers the consumers every later Publish inserts jobs for. Register them at
// composition time, before the bus is shared with concurrent callers.
func (b *Bus) UseConsumers(cs ...Consumer) {
	b.consumers = append(b.consumers, cs...)
}

// queueConsumers inserts, on the publisher's transaction, the job of every consumer that wants
// the event, in one insert.
func (p *txPublisher) queueConsumers(ctx context.Context, env Envelope, ev DomainEvent) error {
	var params []river.InsertManyParams
	for _, c := range p.consumers {
		if c.Wants(env, ev) {
			params = append(params, river.InsertManyParams{Args: c.Job(env)})
		}
	}
	if len(params) == 0 {
		return nil // river refuses an empty insert
	}
	if p.jobs == nil {
		return ErrNoJobQueue
	}
	_, err := p.jobs.InsertManyTx(ctx, p.tx, params)
	return err
}
