package staff

import (
	"context"
	"strconv"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/providerlimit"
	"github.com/getsphericon/sphericon/internal/suspension"
)

// SendCaps is whether the Instance has Send caps, so the console shows a Workspace's.
func (m *Module) SendCaps() providerlimit.SendCaps { return m.sendCaps }

// SentLast24h counts the messages the Workspace w sent over the last 24 hours, the figure the
// console shows next to its Send cap, through a scope over w after its row is loaded
// (docs/architecture.md, scope list item 7) and the same read as /site's (providerlimit).
func (m *Module) SentLast24h(ctx context.Context, w *ent.Workspace) (int, error) {
	if err := m.licensed(); err != nil {
		return 0, err
	}
	return providerlimit.SentLast24h(ctx, m.ent.Scoped(w.ID), m.now())
}

// SetSendCap sets the Workspace's Send cap to perDay messages per rolling 24 hours, or clears it
// with nil, as the Staff member, through the same core mechanism as suspension, audited with the
// old and the new value. It reports whether the value changed. providerlimit.ErrNoSendCaps on an
// Instance without Send caps; an ent not-found error for an unknown Workspace.
func (m *Module) SetSendCap(ctx context.Context, bus *events.Bus, publicID string, by *ent.StaffMember, perDay *int) (*ent.Workspace, bool, error) {
	if err := m.licensed(); err != nil {
		return nil, false, err
	}
	if err := m.sendCaps.Check(); err != nil {
		return nil, false, err
	}
	w, err := m.workspace(ctx, publicID)
	if err != nil {
		return nil, false, err
	}
	changed, err := suspension.SetSendCap(ctx, bus, w.ID, suspension.StaffMember(strconv.FormatInt(by.ID, 10)), perDay)
	if err != nil {
		return nil, false, err
	}
	ws, err := m.ent.Workspace.Get(ctx, w.ID)
	return ws, changed, err
}
