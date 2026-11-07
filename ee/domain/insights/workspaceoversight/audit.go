package workspaceoversight

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// AuditEntries is one page of a Workspace's Audit log, newest first, and the cursor of
// the next page (0 on the last). It rides the same read path as the /site page
// (auditentries.Log.Entries), through a scope over the Workspace the Staff member is looking at,
// so the query carries no hand-written Workspace predicate; ErrWorkspaceNotFound
// when the Workspace does not exist. With WorkspaceDeliverability and SentLast24h it is one of
// the three console reads this package builds a Workspace scope for (docs/architecture.md, scope
// list item 7).
//
// It is gated by the `staff` feature alone: the console is the Staff member's tool and
// the Audit log is one of its cards (ADR 0026). The separate `audit` license decides
// whether entries are recorded at all (the audit consumer discards them without it), so an
// instance with `staff` but no `audit` simply shows an empty log.
func (m *Module) AuditEntries(ctx context.Context, workspaceID string, cursor int64, limit int) (*AuditPage, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	s, err := m.scopeOver(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	rows, next, err := m.audit.Entries(ctx, s, events.AuditFilter{}, cursor, limit)
	if err != nil {
		return nil, err
	}
	return &AuditPage{Entries: rows, Next: next}, nil
}

// AuditPage is a page of a Workspace's Audit log, newest first: its entries and the cursor of the
// next page, 0 on the last one.
type AuditPage struct {
	Entries []*ent.AuditEntry
	Next    int64
}
