package workspaceoversight

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/insights/auditlog"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// AuditEntries is one page of a Workspace's Audit log, newest first, as the console shows it
// (staffPageOf: a Service credential named), with the cursor of the next page ("" on the
// last); auditlog.ErrBadCursor for a cursor the log never handed out. It rides the same read path as the /site page
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
func (m *Module) AuditEntries(ctx context.Context, workspaceID, cursor string, limit int) (auditlog.Page, error) {
	if err := m.licensed(); err != nil {
		return auditlog.Page{}, err
	}
	after, err := auditlog.Cursor(cursor)
	if err != nil {
		return auditlog.Page{}, err
	}
	s, err := m.scopeOver(ctx, workspaceID)
	if err != nil {
		return auditlog.Page{}, err
	}
	page, err := m.audit.Entries(ctx, s, events.AuditFilter{}, after, limit)
	if err != nil {
		return auditlog.Page{}, err
	}
	return staffPageOf(page), nil
}

// staffPageOf is the console's Page of a read: the customer's view of each entry, except that a
// Service credential is named, a machine identity of the platform the customer sees as "Sphericon
// staff" (ADR 0026). Only the console reads it, so it lives here, where no surface of a customer
// can import it.
func staffPageOf(p pagination.IDPage[*ent.AuditEntry]) auditlog.Page {
	out := auditlog.Page{Entries: make([]auditlog.View, len(p.Items))}
	for i, e := range p.Items {
		v := auditlog.ViewOf(e)
		if e.ActorKind == events.ActorService {
			v.ActorKind, v.ActorID, v.ActorName = e.ActorKind, e.ActorID, e.ActorName
		}
		out.Entries[i] = v
	}
	if p.Next > 0 {
		out.Next = auditlog.NextCursor(p.Next)
	}
	return out
}
