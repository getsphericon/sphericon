package suspension

import (
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/workspace"
)

// Is is the one meaning of "suspended": a Workspace whose suspended_at is set (ADR 0007).
// Every test of it, in Go or in a query, goes through Is, Suspended and NotSuspended.
func Is(ws *ent.Workspace) bool { return ws.SuspendedAt != nil }

// Suspended selects suspended Workspaces.
func Suspended() predicate.Workspace { return workspace.SuspendedAtNotNil() }

// NotSuspended selects Workspaces that are not suspended.
func NotSuspended() predicate.Workspace { return workspace.SuspendedAtIsNil() }

// BySystem selects Workspaces the abuse detector suspended (actor `system`).
func BySystem() predicate.Workspace {
	return workspace.SuspendedByKindEQ(workspace.SuspendedByKindSystem)
}
