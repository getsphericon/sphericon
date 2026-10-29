package audit

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/auditapi"
	"github.com/getsphericon/sphericon/internal/events"
)

// exportPageSize is how many entries one query fetches while an export streams.
const exportPageSize = 500

// exportHeader is the CSV column set, one row per Audit entry.
var exportHeader = []string{
	"id", "occurred_at", "actor_kind", "actor_id", "actor_name", "action",
	"target_type", "target_id", "target_name", "diff", "request_id", "ip", "user_agent",
}

// Export records the export of the scoped Workspace's Audit log as an `audit_log.export`
// entry of the scope's actor, whose diff names the filters it is narrowed by (story 19), and
// returns the function that writes the entries matching the filter as CSV. The entry is
// recorded before any row leaves: an error here means nothing was exported. The caller
// has already checked Licensed and the reader's role.
func (l *Log) Export(ctx context.Context, s *ent.Scoped, f events.AuditFilter) (func(w io.Writer) error, error) {
	entry := &events.AuditEntry{Action: events.ActionAuditLogExport, TargetType: "audit_log"}
	if filters := exportFilters(f); len(filters) > 0 {
		entry.Diff = map[string]any{"filter": filters}
	}
	err := l.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		entry.WorkspaceID, entry.Actor = ts.WorkspaceID(), ts.Actor()
		return events.RecordAudit(ctx, pub, entry)
	})
	if err != nil {
		return nil, err
	}
	return func(w io.Writer) error { return l.exportCSV(ctx, s, f, w) }, nil
}

// exportFilters names the filters an export is narrowed by; an unset one is left out.
func exportFilters(f events.AuditFilter) map[string]any {
	filters := map[string]any{}
	if !f.From.IsZero() {
		filters["from"] = f.From.UTC().Format(time.RFC3339)
	}
	if !f.To.IsZero() {
		filters["to"] = f.To.UTC().Format(time.RFC3339)
	}
	for k, v := range map[string]string{
		"actor_kind": f.ActorKind, "actor_id": f.ActorID, "action": f.Action, "target_type": f.TargetType,
		"target_id": f.TargetID, "ip": f.IP, "request_id": f.RequestID,
	} {
		if v != "" {
			filters[k] = v
		}
	}
	return filters
}

// exportCSV streams the scoped Workspace's Audit log entries matching the filter, newest first, to w as CSV: a
// header row, then one row per entry. It pages through Entries, so a long log never
// sits in memory, and it shows a Staff member as "Sphericon staff" like every read surface.
func (l *Log) exportCSV(ctx context.Context, s *ent.Scoped, f events.AuditFilter, w io.Writer) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(exportHeader); err != nil {
		return err
	}
	var cursor int64
	for {
		rows, next, err := l.Entries(ctx, s, f, cursor, exportPageSize)
		if err != nil {
			return err
		}
		for _, e := range rows {
			if err := cw.Write(exportRow(e)); err != nil {
				return err
			}
		}
		cw.Flush()
		if err := cw.Error(); err != nil {
			return err
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}

func exportRow(e *ent.AuditEntry) []string {
	v := auditapi.ViewOf(e)
	diff := ""
	if len(e.Diff) > 0 {
		if b, err := json.Marshal(e.Diff); err == nil {
			diff = string(b)
		}
	}
	row := []string{
		strconv.FormatInt(v.ID, 10), v.OccurredAt.UTC().Format(time.RFC3339), v.ActorKind, valueOrEmpty(v.ActorID), valueOrEmpty(v.ActorName),
		v.Action, v.TargetType, valueOrEmpty(v.TargetID), valueOrEmpty(v.TargetName), diff,
		valueOrEmpty(v.RequestID), valueOrEmpty(v.IP), valueOrEmpty(v.UserAgent),
	}
	for i, cell := range row {
		row[i] = defuseFormula(cell)
	}
	return row
}

// defuseFormula defuses CSV formula injection: a spreadsheet evaluates a cell that
// starts with one of = + - @ (or a tab or carriage return), and actor and target
// names are free text, so such a cell is prefixed with an apostrophe.
func defuseFormula(cell string) string {
	if cell != "" && strings.ContainsRune("=+-@\t\r", rune(cell[0])) {
		return "'" + cell
	}
	return cell
}

func valueOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
