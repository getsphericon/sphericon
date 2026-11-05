package events

import (
	"cmp"
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/postgres"
)

// OrderEvent is a collected order Event as the orders consumer hands it to the OrderRecorder:
// the Event's own properties, the Contact its identity resolved to at ingest (0 for none), its
// time and the id of the bus event, so a redelivered one is read once.
type OrderEvent struct {
	Action     string
	Properties map[string]any
	ContactID  int64
	OccurredAt time.Time
	EventID    string
}

// OrderRecorder reads order Events into Orders (ADR 0064), the attribution of each Order
// included. The orders consumer delegates to it, like the webhooks consumer does to its
// dispatcher: the consumer owns the transaction and the lock, the recorder everything else.
type OrderRecorder interface {
	// IsOrderAction reports whether a collected Event's action is an order Event. The
	// consumer opens no transaction for any other.
	IsOrderAction(action string) bool
	// Prepare does what commits on its own before the Event's transaction opens (the first
	// Order Completed sets the Reporting currency), so the transaction never waits on a second
	// one. It must be safe to repeat: a redelivered Event prepares again.
	Prepare(ctx context.Context, s *ent.Scoped, ev OrderEvent) error
	// Record applies one order Event to the Workspace behind s, inside the transaction.
	Record(ctx context.Context, s *ent.Scoped, ev OrderEvent) error
}

// RecordOrder is the orders consumer's work on one envelope, which its job (internal/jobs)
// runs. An order Event is handed to the
// recorder; every other event is passed over.
func RecordOrder(ctx context.Context, client *ent.Client, recorder OrderRecorder, env Envelope) error {
	ev, err := Decode(env)
	if err != nil {
		return err
	}
	e, ok := ev.(*CollectedEvent)
	if !ok || !recorder.IsOrderAction(e.Action) {
		return nil
	}
	p := e.Project()
	in := OrderEvent{
		Action: p.Action, Properties: p.Properties, ContactID: p.ContactID, OccurredAt: env.OccurredAt,
		EventID: cmp.Or(env.DedupKey, env.ID),
	}
	// The Workspace comes from the event envelope: a consumer has no membership or api token.
	if err := recorder.Prepare(ctx, client.Scoped(env.WorkspaceID), in); err != nil {
		return err
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin order transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// One transaction at a time reads the order Events of a Workspace: an Order Completed that
	// creates an Order and an Event that finds it missing and is held would otherwise cross, and
	// the held one would wait for ever.
	if err := postgres.LockTx(ctx, tx.Client(), advisoryKey("orders", env.WorkspaceID)); err != nil {
		return fmt.Errorf("lock the orders of the Workspace: %w", err)
	}
	if err := recorder.Record(ctx, tx.Client().Scoped(env.WorkspaceID), in); err != nil {
		return err
	}
	return tx.Commit()
}
