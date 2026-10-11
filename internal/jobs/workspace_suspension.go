package jobs

import (
	"context"
	"errors"
	"fmt"

	"github.com/mokevnin/sphericon/ent"
	"github.com/mokevnin/sphericon/ent/membership"
	"github.com/mokevnin/sphericon/ent/workspace"
	"github.com/mokevnin/sphericon/internal/i18n"
	"github.com/mokevnin/sphericon/internal/messaging"
)

// NotifyWorkspaceSuspended tells the workspace owner(s) that outbound sending is
// frozen, why, and who set it (ADR 0007). It goes through the system (platform)
// sender, not the Workspace's own Integration: that is the point of the notice — the
// workspace itself cannot send while suspended — and platform mail is outside
// Outbound send by design (ADR 0015). Pure (no queue) so the CLI and tests call it
// directly. A nil sender or a workspace that is not suspended is a no-op.
func NotifyWorkspaceSuspended(ctx context.Context, client *ent.Client, sender messaging.EmailSender, workspaceID int64) error {
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
	owners, err := client.Scoped(ws.ID).Membership().Query().
		Where(membership.RoleEQ(membership.RoleOwner)).
		WithUser().
		All(ctx)
	if err != nil {
		return err
	}

	reason := ""
	if ws.SuspensionReason != nil {
		reason = *ws.SuspensionReason
	}
	// The owner never sees an Operator's id: the CLI and every Operator are "sphericon staff".
	actorKey := "email.workspace_suspended.actor_staff"
	if ws.SuspendedByKind != nil && *ws.SuspendedByKind == workspace.SuspendedByKindSystem {
		actorKey = "email.workspace_suspended.actor_system"
	}
	data := map[string]any{"Workspace": ws.Name, "Reason": reason, "Actor": i18n.T(actorKey, nil)}
	subject := i18n.T("email.workspace_suspended.subject", data)
	body := i18n.T("email.workspace_suspended.body", data)

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
