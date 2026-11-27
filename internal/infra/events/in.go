package events

import (
	"context"
	"errors"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// ErrNotInBusTx is returned by PublishIn, EnqueueIn and RecordAuditIn for a client that is not bound to a
// transaction the bus opened: one on the pool, in ent's own Tx, or in another bus's transaction.
// The bus never opens a transaction for it.
var ErrNotInBusTx = errors.New("events: the client is not bound to a transaction of this bus")

// ErrNotPurging is returned by PurgeJobsIn for a scope bound to a transaction of the bus that is
// not an Erasure's (WithinScopedPurgeTx): no other transaction purges the queues.
var ErrNotPurging = errors.New("events: the scope is not bound to a purging transaction")

// InTx is a client bound to a transaction: the *ent.Client a Within… method hands out, and the
// *ent.Scoped, *ent.WorkspaceLocked and *ent.DestinationLocked bound to it.
type InTx interface {
	TxDriver() (ent.TxBound, bool)
}

// PublishIn publishes ev in the transaction in is bound to, through that transaction's own
// publisher (the one its Within… method hands out), so the event exists iff the transaction
// commits. It is how a domain type that took the bus in its constructor publishes in the
// transaction of the scope its operation is given (ADR 0167).
func (b *Bus) PublishIn(ctx context.Context, in InTx, ev DomainEvent) error {
	pub, err := b.publisherOf(in)
	if err != nil {
		return err
	}
	return pub.Publish(ctx, ev)
}

// RecordAuditIn records the explicit Audit entry in the transaction in is bound to: RecordAudit
// through that transaction's own publisher (PublishIn). It is how a domain type that took the bus
// in its constructor records an entry in the transaction of the scope its operation is given.
func (b *Bus) RecordAuditIn(ctx context.Context, in InTx, entry *AuditEntry) error {
	pub, err := b.publisherOf(in)
	if err != nil {
		return err
	}
	return RecordAudit(ctx, pub, entry)
}

// EnqueueIn queues a job in the transaction in is bound to (PublishIn, Enqueuer).
func (b *Bus) EnqueueIn(ctx context.Context, in InTx, args river.JobArgs, opts *river.InsertOpts) error {
	pub, err := b.publisherOf(in)
	if err != nil {
		return err
	}
	return pub.Enqueue(ctx, args, opts)
}

// PurgeJobsIn deletes the river jobs of kind of the scope's Workspace whose args integer field key
// is one of ids (QueuePurger.PurgeJobs), in the Erasure's transaction s is bound to, so the purge
// commits or rolls back with it. It is how an owner's unchecked cancellation, which took the bus in
// its constructor, purges its pending work in the transaction of the scope it is given (ADR 0167).
// A scope bound to no transaction of the bus is ErrNotInBusTx, one bound to a transaction that
// WithinScopedPurgeTx did not open ErrNotPurging: only the Erasure purges (ADR 0021).
func (b *Bus) PurgeJobsIn(ctx context.Context, s *ent.Scoped, key jobkind.Key, ids []int64) error {
	pub, err := b.publisherOf(s)
	if err != nil {
		return err
	}
	if !pub.purging {
		return ErrNotPurging
	}
	return pub.PurgeJobs(ctx, s.WorkspaceID(), key, ids)
}

// publisherOf recovers the publisher Bus.within built for the transaction in is bound to.
func (b *Bus) publisherOf(in InTx) (*txPublisher, error) {
	d, ok := in.TxDriver()
	if !ok {
		return nil, ErrNotInBusTx
	}
	drv, ok := d.(*txDriver)
	if !ok || drv.pub == nil || drv.pub.bus != b {
		return nil, ErrNotInBusTx
	}
	return drv.pub, nil
}
