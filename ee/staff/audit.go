package staff

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
)

// AuditEntries is one page of a Workspace's Audit log, newest first, and the cursor of
// the next page (0 on the last). It rides the same read path as the /site page
// (audit.Log.Entries), through a scope over the Workspace the Staff member is looking at,
// so the query carries no hand-written Workspace predicate; an ent not-found error
// when the Workspace does not exist. With WorkspaceDeliverability it is one of the
// two places this package builds a Workspace scope (AGENTS.md, scope list item 7).
//
// It is gated by the `staff` feature alone: the console is the Staff member's tool and
// the Audit log is one of its cards (ADR 0026). The separate `audit` license decides
// whether entries are recorded at all (the subscriber discards them without it), so an
// instance with `staff` but no `audit` simply shows an empty log.
func (m *Module) AuditEntries(ctx context.Context, workspaceID, cursor int64, limit int) ([]*ent.AuditEntry, int64, error) {
	if err := m.licensed(); err != nil {
		return nil, 0, err
	}
	s, err := m.scopeOver(ctx, workspaceID)
	if err != nil {
		return nil, 0, err
	}
	return m.audit.Entries(ctx, s, events.AuditFilter{}, cursor, limit)
}
