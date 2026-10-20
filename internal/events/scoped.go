package events

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// WithinScopedTx is WithinTx for a caller that holds a Workspace-scoped client: fn
// receives a scoped client over the same Workspace but bound to the transaction, so
// a module can write through it without ever naming the Workspace as an int64. The
// scope keeps its actor, and an audited write through it joins this transaction (its
// entry is published here) instead of opening another one (ADR 0022).
func (b *Bus) WithinScopedTx(ctx context.Context, s *ent.Scoped, fn func(ts *ent.Scoped, pub Publisher) error) error {
	return b.WithinTx(ctx, func(tx *ent.Client, pub Publisher) error {
		return fn(s.InTx(tx, auditPublisher{pub: pub}), pub)
	})
}

// WithinHandOverTx is WithinScopedTx for a hand-over between two Workspaces (a Join
// domain moving to the Workspace that now proves it): fn receives to, the caller's scope
// bound to the transaction, and from, a scope over the Workspace named by fromID acting as
// the system, in the same transaction, so both sides commit or neither does. The
// hand-over is the one place a transaction is scoped to a Workspace other than the caller's.
func (b *Bus) WithinHandOverTx(ctx context.Context, s *ent.Scoped, fromID int64, fn func(to, from *ent.Scoped) error) error {
	return b.WithinTx(ctx, func(tx *ent.Client, pub Publisher) error {
		ap := auditPublisher{pub: pub}
		from := tx.Scoped(fromID).As(Actor{Kind: ActorSystem}, b).InTx(tx, ap)
		return fn(s.InTx(tx, ap), from)
	})
}

// WithinAuditTx makes the Bus the transaction opener of an audited scope
// (ent.AuditOpener): the wrapper of an audited entity runs its mutation in a
// transaction opened here and publishes the entry in it.
func (b *Bus) WithinAuditTx(ctx context.Context, s *ent.Scoped, fn func(ts *ent.Scoped) error) error {
	return b.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ Publisher) error { return fn(ts) })
}

// Act returns the scope with the actor of its writes (ADR 0022). Every construction
// point of a scope sets it; a scope without an actor, or with ActorIngest, is never
// audited.
func (b *Bus) Act(s *ent.Scoped, actor Actor) *ent.Scoped { return s.As(actor, b) }

// Ingest returns the scope of a secret-built caller (collect key, provider hook,
// signed link, bootstrap token): its writes are never audited.
func Ingest(s *ent.Scoped) *ent.Scoped { return s.As(Actor{Kind: ActorIngest}, nil) }

// Unaudited returns the scope with its writes left out of the Audit log, for a write that
// is not a configuration change by the actor: a bulk import records one entry for the
// whole import (contacts.RecordImport), and applying a Tag to a Contact is a data-plane
// edge, not a field of the Contact.
func Unaudited(s *ent.Scoped) *ent.Scoped { return s.As(Actor{Kind: ActorIngest}, nil) }

// auditPublisher publishes the scoped client's audit changes through the seam, in the
// transaction of the underlying Publisher.
type auditPublisher struct{ pub Publisher }

func (p auditPublisher) PublishAudit(ctx context.Context, workspaceID int64, c ent.AuditChange) error {
	return RecordAudit(ctx, p.pub, &AuditEntry{
		WorkspaceID: workspaceID,
		Actor:       c.Actor,
		Action:      c.Action,
		TargetType:  c.TargetType,
		TargetID:    c.TargetID,
		TargetName:  c.TargetName,
		Diff:        c.Diff,
	})
}

// PurgingPublisher is the Publisher of a transaction that may also clear the internal
// queues (see QueuePurger). Only WithinScopedPurgeTx hands one out.
type PurgingPublisher interface {
	Publisher
	QueuePurger
}

// WithinScopedPurgeTx is WithinScopedTx for the one caller that clears the internal
// queues in the same transaction: Erasure (ADR 0021). Asking for the purging
// publisher here, not by type assertion on a Publisher, makes it a compile-time
// dependency.
func (b *Bus) WithinScopedPurgeTx(ctx context.Context, s *ent.Scoped, fn func(ts *ent.Scoped, pub PurgingPublisher) error) error {
	return b.within(ctx, func(tx *ent.Client, pub *txPublisher) error {
		// The scope keeps its actor, so deleting the Contact is audited (id only) in the
		// erasure's own transaction (ADR 0022).
		return fn(s.InTx(tx, auditPublisher{pub: pub}), pub)
	})
}
