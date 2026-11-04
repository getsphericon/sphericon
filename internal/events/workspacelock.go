package events

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/postgres"
)

// WithinWorkspaceLockTx is WithinScopedTx holding the Workspace's lock (ADR 0099): its first
// statement takes the Workspace's transaction-scoped advisory lock, and the lock is held until the
// transaction ends. A removal that checks Live use (archiving or deleting a Segment, an
// Integration, a Sending domain, a Tag) and a write that creates it (scheduling a Broadcast,
// Publish, saving a Segment or a versioned definition) run in such a transaction, so one waits for
// the other and then reads what it committed: the check never passes on work that becomes live
// before the removal commits.
//
// The lock comes before any row lock the transaction takes, so two of these never wait for each
// other in a cycle. A hot path (a Rule firing, an Enrollment) never takes it: it locks the row it
// acts on and checks it again under that lock.
func (b *Bus) WithinWorkspaceLockTx(ctx context.Context, s *ent.Scoped, fn func(ts *ent.Scoped, pub Publisher) error) error {
	return b.within(ctx, func(tx *ent.Client, pub *txPublisher) error {
		if err := lockWorkspace(ctx, tx, s); err != nil {
			return err
		}
		return fn(s.InTx(tx, auditPublisher{pub: pub}), pub)
	})
}

// WithinWorkspaceLockEnqueueTx is WithinWorkspaceLockTx for a caller that also queues a job in the
// same transaction (WithinScopedEnqueueTx): a Broadcast's claim and its send job.
func (b *Bus) WithinWorkspaceLockEnqueueTx(ctx context.Context, s *ent.Scoped, fn func(ts *ent.Scoped, pub Publisher, q Enqueuer) error) error {
	return b.within(ctx, func(tx *ent.Client, pub *txPublisher) error {
		if err := lockWorkspace(ctx, tx, s); err != nil {
			return err
		}
		return fn(s.InTx(tx, auditPublisher{pub: pub}), pub, txQueue{jobs: pub.jobs, tx: pub.tx})
	})
}

func lockWorkspace(ctx context.Context, tx *ent.Client, s *ent.Scoped) error {
	if err := postgres.LockTx(ctx, tx, workspaceLockKey(s.WorkspaceID())); err != nil {
		return fmt.Errorf("lock the Workspace: %w", err)
	}
	return nil
}

// workspaceLockKey is the advisory lock of one Workspace's removals and the writes that create
// Live use. Two Workspaces whose keys collide only take turns.
func workspaceLockKey(workspaceID int64) int64 {
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "workspace:%d", workspaceID)
	return int64(h.Sum32())
}
