package testhelper

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent/workspace"
)

// SuspendWorkspace leaves a Workspace suspended at `at` the way internal/suspension does: the
// actor and the reason are written with suspended_at, as the table's check requires.
func (env *TestEnv) SuspendWorkspace(ctx context.Context, id int64, at time.Time) {
	env.DB.Workspace.UpdateOneID(id).
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
