package auditentries

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"iter"
	"strconv"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/insights/auditlog"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// exportPageSize is how many entries one query fetches while an export streams.
const exportPageSize = 500

// exportHeader is the CSV column set, one row per Audit entry.
var exportHeader = []string{
	"id", "occurred_at", "actor_kind", "actor_id", "actor_name", "action",
	"target_type", "target_id", "target_name", "diff", "request_id", "ip", "user_agent",
}

// targetAuditLog is the target of an export of the Audit log, which no entity holds, and diffFilter
// the Diff key of the filters it is narrowed by.
const (
	targetAuditLog = "audit_log"
	diffFilter     = "filter"
)

// Export records the export of the scoped Workspace's Audit log as an `audit_log.export`
// entry of the scope's actor, whose diff names the filters it is narrowed by (story 19), and
// returns the entries matching the filter as CSV, yielded in chunks the driver streams
// (ADR 0111). The entry is recorded before any row leaves: an error here means nothing was
// exported. The caller has already checked Licensed and the reader's role.
func (l *Log) Export(ctx context.Context, s *ent.Scoped, f events.AuditFilter) (iter.Seq2[[]byte, error], error) {
	entry := &events.AuditEntry{Action: events.ActionAuditLogExport, TargetType: targetAuditLog}
	if filters := exportFilters(f); len(filters) > 0 {
		entry.Diff = map[string]any{diffFilter: filters}
	}
	err := l.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		entry.WorkspaceID, entry.Actor = ts.WorkspaceID(), ts.Actor()
		return events.RecordAudit(ctx, pub, entry)
	})
	if err != nil {
		return nil, err
	}
	return l.exportCSV(ctx, s, f), nil
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

// exportCSV yields the scoped Workspace's Audit log entries matching the filter, newest first,
// as CSV: a header row, then the rows of each page of entries, a chunk per page. It pages
// through Entries, so a long log never sits in memory, and it shows a Staff member as
// "Sphericon staff" like every read surface. A failed read ends it with the error.
func (l *Log) exportCSV(ctx context.Context, s *ent.Scoped, f events.AuditFilter) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		if !yieldCSV(yield, [][]string{exportHeader}) {
			return
		}
		var cursor int64
		for {
			page, err := l.Entries(ctx, s, f, cursor, exportPageSize)
			if err != nil {
				yield(nil, err)
				return
			}
			rows := make([][]string, 0, len(page.Items))
			for _, e := range page.Items {
				rows = append(rows, exportRow(e))
			}
			if !yieldCSV(yield, rows) || page.Next == 0 {
				return
			}
			cursor = page.Next
		}
	}
}

// yieldCSV yields rows as one chunk of CSV; false once the reader stopped or the rows could not
// be encoded.
func yieldCSV(yield func([]byte, error) bool, rows [][]string) bool {
	var buf bytes.Buffer
	if err := csv.NewWriter(&buf).WriteAll(rows); err != nil {
		yield(nil, err)
		return false
	}
	return yield(buf.Bytes(), nil)
}

func exportRow(e *ent.AuditEntry) []string {
	v := auditlog.ViewOf(e)
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
