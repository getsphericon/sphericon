package events

import (
	"context"
	"errors"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
)

// ErrNotInBusTx is returned by PublishIn and EnqueueIn for a client that is not bound to a
// transaction the bus opened: one on the pool, in ent's own Tx, or in another bus's transaction.
// The bus never opens a transaction for it.
var ErrNotInBusTx = errors.New("events: the client is not bound to a transaction of this bus")

// InTx is a client bound to a transaction: the *ent.Client a Within… method hands out, and the
// *ent.Scoped and *ent.WorkspaceLocked bound to it.
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

// EnqueueIn queues a job in the transaction in is bound to (PublishIn, Enqueuer).
func (b *Bus) EnqueueIn(ctx context.Context, in InTx, args river.JobArgs, opts *river.InsertOpts) error {
	pub, err := b.publisherOf(in)
	if err != nil {
		return err
	}
	return pub.Enqueue(ctx, args, opts)
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
