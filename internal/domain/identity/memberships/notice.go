package memberships

import (
	"context"
	"log/slog"

	"github.com/getsphericon/sphericon/internal/kernel/i18n"
)

// The notices to the Owners of a Workspace: the one seam of the modules that tell them something
// (Join domains, Sending domains, webhooks, suspension, the abuse detector). A Notify delivers a
// rendered notice, and Tell and TellBestEffort render one from its Notice and data; OwnerNotifier
// is the Notify that mails the Owners.

// Notify mails the Owners of a Workspace through the system sender. A nil Notify sends nothing
// (no platform sender is configured).
type Notify func(ctx context.Context, workspaceID int64, subject, body string) error

// Notice is the subject and the body of a notice, as messages of the catalog.
type Notice struct {
	Subject, Body i18n.ID
}

// Tell renders the notice with data and sends it to the Owners of the Workspace. A nil Notify
// does nothing.
func (n Notify) Tell(ctx context.Context, workspaceID int64, notice Notice, data map[string]any) error {
	if n == nil {
		return nil
	}
	return n(ctx, workspaceID, i18n.T(notice.Subject, data), i18n.T(notice.Body, data))
}

// TellBestEffort is Tell for a notice whose failure must not fail the work that caused it: the
// failure is logged with attrs and not returned.
func (n Notify) TellBestEffort(ctx context.Context, workspaceID int64, notice Notice, data map[string]any, attrs ...any) {
	if err := n.Tell(ctx, workspaceID, notice, data); err != nil {
		slog.WarnContext(ctx, "owner notice failed", append([]any{"notice", notice.Subject.Key(), "workspace_id", workspaceID, "err", err}, attrs...)...)
	}
}
