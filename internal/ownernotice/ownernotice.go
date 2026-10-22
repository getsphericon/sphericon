// Package ownernotice is the one seam of the modules that tell the Owners of a Workspace something
// (Join domains, Sending domains, webhooks, suspension): a Notify delivers a rendered notice, and
// Tell and TellBestEffort render one from its i18n key and data. It is a leaf, so the modules that
// accounts itself imports can share it; accounts.OwnerNotifier is the Notify that mails the Owners.
package ownernotice

import (
	"context"
	"log/slog"

	"github.com/getsphericon/sphericon/internal/i18n"
)

// Notify mails the Owners of a Workspace through the system sender. A nil Notify sends nothing
// (no platform sender is configured).
type Notify func(ctx context.Context, workspaceID int64, subject, body string) error

// Tell renders the notice `<key>.subject` and `<key>.body` with data and sends it to the Owners of
// the Workspace. A nil Notify does nothing.
func (n Notify) Tell(ctx context.Context, workspaceID int64, key string, data map[string]any) error {
	if n == nil {
		return nil
	}
	return n(ctx, workspaceID, i18n.T(key+".subject", data), i18n.T(key+".body", data))
}

// TellBestEffort is Tell for a notice whose failure must not fail the work that caused it: the
// failure is logged with attrs and not returned.
func (n Notify) TellBestEffort(ctx context.Context, workspaceID int64, key string, data map[string]any, attrs ...any) {
	if err := n.Tell(ctx, workspaceID, key, data); err != nil {
		slog.WarnContext(ctx, "owner notice failed", append([]any{"notice", key, "workspace_id", workspaceID, "err", err}, attrs...)...)
	}
}
