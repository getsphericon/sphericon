package events

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// WithinWorkspaceLockTx is WithinScopedTx holding the Workspace's lock (ADR 0099, ADR 0108): its
// first statement takes the Workspace's transaction-scoped advisory lock, held until the
// transaction ends, and fn gets the scope that stands for it, *ent.WorkspaceLocked. What needs the
// lock takes that type: the readers of Live use and Definition use (the Removal check's), the
// unchecked removals behind their check, and the writes that move Live use. One waits for the
// other and then reads what it committed, so a check never passes on work that becomes live
// before the removal commits.
//
// The lock comes before any row lock the transaction takes, so two of these never wait for each
// other in a cycle; this bus is the one caller of ent.Scoped.LockWorkspace (forbidigo). A hot path
// (a Rule firing, an Enrollment) never takes it: it locks the row it acts on and checks it again
// under that lock.
func (b *Bus) WithinWorkspaceLockTx(ctx context.Context, s *ent.Scoped, fn func(l *ent.WorkspaceLocked, pub Publisher) error) error {
	return b.within(ctx, func(tx *ent.Client, pub *txPublisher) error {
		l, err := s.InTx(tx, auditPublisher{pub: pub}).LockWorkspace(ctx)
		if err != nil {
			return err
		}
		return fn(l, pub)
	})
}

// WithinWorkspaceLockEnqueueTx is WithinWorkspaceLockTx for a caller that also queues a job in the
// same transaction (WithinScopedEnqueueTx): a Broadcast's claim and its send job, an Integration's
// save and its quota lookup.
func (b *Bus) WithinWorkspaceLockEnqueueTx(ctx context.Context, s *ent.Scoped, fn func(l *ent.WorkspaceLocked, pub Publisher, q Enqueuer) error) error {
	return b.within(ctx, func(tx *ent.Client, pub *txPublisher) error {
		l, err := s.InTx(tx, auditPublisher{pub: pub}).LockWorkspace(ctx)
		if err != nil {
			return err
		}
		return fn(l, pub, txQueue{jobs: pub.jobs, tx: pub.tx})
	})
}
