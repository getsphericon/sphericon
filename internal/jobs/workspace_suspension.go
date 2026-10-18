package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/messaging"
)

// NotifyWorkspaceSuspended tells the workspace owner(s) that outbound sending is
// frozen, why, and who set it (ADR 0007). It goes through the system (platform)
// sender, not the Workspace's own Integration: that is the point of the notice — the
// workspace itself cannot send while suspended — and platform mail is outside
// Outbound send by design (ADR 0015). Pure (no queue) so the CLI and tests call it
// directly. supportEmail, when set, is where the owner appeals. A nil sender or a workspace that is not suspended is a no-op.
func NotifyWorkspaceSuspended(ctx context.Context, client *ent.Client, sender messaging.EmailSender, supportEmail string, workspaceID int64) error {
	if sender == nil {
		return nil
	}
	ws, err := client.Workspace.Get(ctx, workspaceID)
	if err != nil {
		return err
	}
	if ws.SuspendedAt == nil {
		return nil
	}
	reason := ""
	if ws.SuspensionReason != nil {
		reason = *ws.SuspensionReason
	}
	// The owner never sees a Staff member's id: the CLI and every Staff member are "sphericon staff".
	actorKey := "email.workspace_suspended.actor_staff"
	if ws.SuspendedByKind != nil && *ws.SuspendedByKind == workspace.SuspendedByKindSystem {
		actorKey = "email.workspace_suspended.actor_system"
	}
	data := map[string]any{"Workspace": ws.Name, "Reason": reason, "Actor": i18n.T(actorKey, nil), "Support": supportEmail}
	subject := i18n.T("email.workspace_suspended.subject", data)
	body := i18n.T("email.workspace_suspended.body", data)

	if err := NotifyWorkspaceOwners(ctx, client, sender, ws.ID, subject, body); err != nil {
		return err
	}
	return client.Workspace.UpdateOneID(ws.ID).SetAbuseFreezeNotifiedAt(time.Now()).Exec(ctx)
}

// NotifyWorkspaceOwners sends one platform email to every owner of the Workspace through
// the system sender, never the Workspace's own Integration (a suspended Workspace cannot
// send). It tries every owner and joins the failures.
func NotifyWorkspaceOwners(ctx context.Context, client *ent.Client, sender messaging.EmailSender, workspaceID int64, subject, body string) error {
	owners, err := client.Scoped(workspaceID).Membership().Query().
		Where(membership.RoleEQ(membership.RoleOwner)).
		WithUser().
		All(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, m := range owners {
		u := m.Edges.User
		if u == nil || u.Email == "" {
			continue
		}
		if _, serr := sender.Send(ctx, messaging.EmailMessage{To: u.Email, Subject: subject, Text: body}); serr != nil {
			errs = append(errs, fmt.Errorf("notify %s: %w", u.Email, serr))
		}
	}
	return errors.Join(errs...)
}
