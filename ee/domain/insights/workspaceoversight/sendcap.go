package workspaceoversight

import (
	"context"
	"strconv"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
)

// SendCapChange is a change of a Workspace's Send cap: the Workspace as it stands after it,
// whether the value changed, and its sends over the last 24 hours, shown next to the cap.
type SendCapChange struct {
	WorkspaceChange

	SentLast24h int
}

// sentLast24h counts the messages the Workspace w sent over the last 24 hours, the figure the
// console shows next to its Send cap, through a scope over w after its row is loaded
// (docs/architecture/workspace-scoping.md, scope list item 7) and the same read as /site's (providerlimit).
func (m *Module) sentLast24h(ctx context.Context, w *ent.Workspace) (int, error) {
	return providerlimit.SentLast24h(ctx, m.ent.Scoped(w.ID), m.now())
}

// SetSendCap sets the Workspace's Send cap to perDay messages per rolling 24 hours, or clears it
// with nil, as the Staff member, through the same core mechanism as suspension, audited with the
// old and the new value. It reports whether the value changed, and the Workspace's sends over the
// last 24 hours. providerlimit.ErrNoSendCaps on an Instance without Send caps;
// ErrWorkspaceNotFound for an unknown Workspace.
func (m *Module) SetSendCap(ctx context.Context, publicID string, by *ent.StaffMember, perDay *int) (*SendCapChange, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	if err := m.sendCaps.Check(); err != nil {
		return nil, err
	}
	w, err := m.workspace(ctx, publicID)
	if err != nil {
		return nil, err
	}
	changed, err := suspension.SetSendCap(ctx, m.bus, w.ID, suspension.StaffMember(strconv.FormatInt(by.ID, 10)), perDay)
	if err != nil {
		return nil, err
	}
	done, err := m.changeOf(ctx, w.ID, changed)
	if err != nil {
		return nil, err
	}
	sent, err := m.sentLast24h(ctx, done.Workspace)
	if err != nil {
		return nil, err
	}
	return &SendCapChange{WorkspaceChange: *done, SentLast24h: sent}, nil
}
