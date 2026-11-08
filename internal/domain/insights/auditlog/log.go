package auditlog

import (
	"context"
	"iter"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

var (
	// ErrUnlicensed refuses the Audit log on an Instance whose License does not include it
	// (ADR 0014, ADR 0022).
	ErrUnlicensed = validation.Unlicensed("auditlog: the audit log needs an Enterprise license", i18n.ErrorsAuditUnlicensed)
	// ErrRetentionUnlicensed refuses the Audit log's retention window on an Instance whose License
	// does not include it.
	ErrRetentionUnlicensed = validation.Unlicensed("auditlog: audit retention needs an Enterprise license", i18n.ErrorsAuditRetentionUnlicensed)
)

// Reader is the Enterprise Audit log's read side (ee/domain/insights/auditentries), the one seam of
// it the core knows: what the License includes and the reads of a Workspace's entries. Unlicensed,
// Licensed and RetentionLicensed are false.
type Reader interface {
	Licensed() bool
	RetentionLicensed() bool
	// Entries returns up to limit entries matching the filter, newest first, preceding the cursor
	// entry id (0 = from the newest), and the next page's cursor (0 = last page).
	Entries(ctx context.Context, s *ent.Scoped, f events.AuditFilter, cursor int64, limit int) (pagination.IDPage[*ent.AuditEntry], error)
	// Facets returns the actors, actions and target types that occur in the log, sorted.
	Facets(ctx context.Context, s *ent.Scoped) (Facets, error)
	// Export records the export as an Audit entry of the scope's actor, then returns the entries
	// matching the filter, newest first, as CSV chunks the driver streams.
	Export(ctx context.Context, s *ent.Scoped, f events.AuditFilter) (iter.Seq2[[]byte, error], error)
}

// Log serves a Workspace's Audit log to the surfaces that show it (/site, /api), refused as
// ErrUnlicensed (its retention window as ErrRetentionUnlicensed) when the License lacks it. The
// reader's Role is checked by the surface's access before; a nil Log, or one over no Reader, is
// unlicensed.
type Log struct {
	reader     Reader
	workspaces *workspaces.Module
}

// New builds the Log over the Enterprise Edition's reader (nil for none) and the Workspaces, which
// hold the retention window.
func New(reader Reader, ws *workspaces.Module) *Log { return &Log{reader: reader, workspaces: ws} }

// Query is what a page of the log is asked with: the filter, the cursor a previous page handed out
// ("" for the newest entries) and the page's size.
type Query struct {
	Filter events.AuditFilter
	Cursor string
	Limit  int
}

// Page is one page of the scoped Workspace's entries matching the query, newest first, as a
// customer sees them: a Staff member only as events.StaffLabel. ErrBadCursor for a cursor the log
// never handed out.
func (l *Log) Page(ctx context.Context, s *ent.Scoped, q Query) (Page, error) {
	if !l.Licensed() {
		return Page{}, ErrUnlicensed
	}
	cursor, err := Cursor(q.Cursor)
	if err != nil {
		return Page{}, err
	}
	page, err := l.reader.Entries(ctx, s, q.Filter, cursor, q.Limit)
	if err != nil {
		return Page{}, err
	}
	return pageOf(page), nil
}

// Facets are the values that occur in the scoped Workspace's log, the choices of its filter.
func (l *Log) Facets(ctx context.Context, s *ent.Scoped) (Facets, error) {
	if !l.Licensed() {
		return Facets{}, ErrUnlicensed
	}
	return l.reader.Facets(ctx, s)
}

// Export records the export as an Audit entry of the scope's actor, then returns the scoped
// Workspace's entries matching the filter, newest first, as CSV chunks the driver streams.
func (l *Log) Export(ctx context.Context, s *ent.Scoped, f events.AuditFilter) (iter.Seq2[[]byte, error], error) {
	if !l.Licensed() {
		return nil, ErrUnlicensed
	}
	return l.reader.Export(ctx, s, f)
}

// Retention is the scoped Workspace's retention window in days, nil while its entries are kept.
func (l *Log) Retention(ctx context.Context, s *ent.Scoped) (*int, error) {
	if !l.retentionLicensed() {
		return nil, ErrRetentionUnlicensed
	}
	ws, err := l.workspaces.Get(ctx, s)
	if err != nil {
		return nil, err
	}
	return ws.RetentionDays, nil
}

// SetRetention sets the scoped Workspace's retention window, or clears it with nil, as the scope's
// actor: a change is recorded as a `workspace.update` Audit entry.
func (l *Log) SetRetention(ctx context.Context, s *ent.Scoped, days *int) error {
	if !l.retentionLicensed() {
		return ErrRetentionUnlicensed
	}
	return l.workspaces.SetAuditRetention(ctx, s, s.Actor(), days)
}

// Licensed reports whether the Instance's License includes the Audit log.
func (l *Log) Licensed() bool { return l != nil && l.reader != nil && l.reader.Licensed() }

func (l *Log) retentionLicensed() bool {
	return l != nil && l.reader != nil && l.reader.RetentionLicensed()
}
