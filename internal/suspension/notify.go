package suspension

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/ownernotice"
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

// NotifyOwners tells the Workspace's Owners that outbound sending is frozen, why, and who set
// it (ADR 0007). notify is the ownernotice.Notify seam: the platform's system sender, not the
// Workspace's own Integration, because the Workspace cannot send while suspended (ADR 0015).
// supportEmail, when set, is where the Owner appeals. A nil notify or a Workspace that is not
// suspended is a no-op. It stamps the abuse freeze notice marker once the notice went.
func NotifyOwners(ctx context.Context, client *ent.Client, notify ownernotice.Notify, supportEmail string, workspaceID int64) error {
	if notify == nil {
		return nil
	}
	ws, err := client.Workspace.Get(ctx, workspaceID)
	if err != nil {
		return err
	}
	if !Is(ws) {
		return nil
	}
	reason := ""
	if ws.SuspensionReason != nil {
		reason = *ws.SuspensionReason
	}
	// The owner never sees a Staff member's id: the CLI and every Staff member are "Sphericon staff".
	actorKey := "email.workspace_suspended.actor_staff"
	if ws.SuspendedByKind != nil && *ws.SuspendedByKind == workspace.SuspendedByKindSystem {
		actorKey = "email.workspace_suspended.actor_system"
	}
	data := map[string]any{"Workspace": ws.Name, "Reason": reason, "Actor": i18n.T(actorKey, nil), "Support": supportEmail}
	if err := notify.Tell(ctx, ws.ID, "email.workspace_suspended", data); err != nil {
		return err
	}
	return client.Workspace.UpdateOneID(ws.ID).SetAbuseFreezeNotifiedAt(time.Now()).Exec(ctx)
}
