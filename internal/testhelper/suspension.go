package testhelper

import (
	"context"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspace"
)

// SuspendWorkspace leaves a Workspace suspended at `at` the way internal/domain/identity/suspension does: the
// actor and the reason are written with suspended_at, as the table's check requires.
func (env *TestEnv) SuspendWorkspace(ctx context.Context, id int64, at time.Time) {
	env.DB.Workspace.Update().Where(workspace.ID(id)).
		SetSuspendedAt(at).
		SetSuspendedByKind(workspace.SuspendedByKindSystem).
		SetSuspensionReason("test").
		ExecX(ctx)
}

// UnsuspendWorkspace lifts what SuspendWorkspace set, clearing the actor and the reason with it.
func (env *TestEnv) UnsuspendWorkspace(ctx context.Context, id int64) {
	env.DB.Workspace.UpdateOneID(id).
		ClearSuspendedAt().
		ClearSuspendedByKind().
		ClearSuspendedByID().
		ClearSuspensionReason().
		ExecX(ctx)
}

// DeleteWorkspace puts a Workspace into deletion at `at`, as its Owner's delete does: the write goes
// through Feed, since the Hooks record a Workspace's deletion, which env.DB refuses outside a
// transaction (internal/AGENTS.md).
func (env *TestEnv) DeleteWorkspace(tb testing.TB, id int64, at time.Time) {
	tb.Helper()
	env.Feed(tb, func(tx *ent.Client) error {
		return tx.Workspace.UpdateOneID(id).SetDiscardedAt(at).Exec(tb.Context())
	})
}
