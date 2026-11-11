package events

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/postgres"
)

// WithinWorkspaceLockTx is WithinScopedTx holding the Workspace's lock (ADR 0099, ADR 0108): its
// first statement takes the Workspace's transaction-scoped advisory lock, held until the
// transaction ends, and fn gets the scope that stands for it, *ent.WorkspaceLocked, which what needs
// the lock takes (the Workspace lock section of docs/architecture/lifecycles.md). A removal's check
// and a write that moves the use it checks wait for each other, and the one that waits reads what
// the other committed: the check never passes on work that becomes live before the removal commits.
//
// The lock comes before any row lock the transaction takes, so two of these never wait for each
// other in a cycle; this bus is the one caller of ent.Scoped.LockWorkspace (forbidigo). A hot path
// (a Rule firing, an Enrollment) never takes it: it locks the row it acts on and checks it again
// under that lock.
func (b *Bus) WithinWorkspaceLockTx(ctx context.Context, s *ent.Scoped, fn func(l *ent.WorkspaceLocked, pub Publisher) error) error {
	return b.within(ctx, func(tx *ent.Client, pub *txPublisher) error {
		l, err := s.InTx(tx, auditPublisher{pub: pub}).LockWorkspace(ctx, lockWorkspace)
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
		l, err := s.InTx(tx, auditPublisher{pub: pub}).LockWorkspace(ctx, lockWorkspace)
		if err != nil {
			return err
		}
		return fn(l, pub, txQueue{jobs: pub.jobs, tx: pub.tx})
	})
}

// lockWorkspace takes the Workspace's lock on the transaction tx.
func lockWorkspace(ctx context.Context, tx *ent.Client, workspaceID int64) error {
	if err := postgres.LockTx(ctx, tx, advisoryKey("workspace", workspaceID)); err != nil {
		return fmt.Errorf("lock the Workspace: %w", err)
	}
	return nil
}

// advisoryKey is the key of one Workspace's advisory lock of a kind: "workspace" for its removals
// and the writes that move Live use, "orders" for its order Events. Each kind hashes apart from the
// others, and two Workspaces whose keys collide only take turns.
func advisoryKey(kind string, workspaceID int64) int64 {
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%s:%d", kind, workspaceID)
	return int64(h.Sum32())
}
