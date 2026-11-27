package auditentries

import (
	"context"
	"encoding/json"
	"iter"
	"strconv"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/insights/auditlog"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/csvfile"
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
// as a CSV file (internal/kernel/csvfile): a header row, then the rows of each page of entries, a
// chunk per page. It pages through Entries, so a long log never sits in memory, and it shows a
// Staff member as "Sphericon staff" like every read surface. A failed read ends it with the error.
func (l *Log) exportCSV(ctx context.Context, s *ent.Scoped, f events.AuditFilter) iter.Seq2[[]byte, error] {
	return csvfile.Chunks(func(yield func([][]string, error) bool) {
		if !yield([][]string{exportHeader}, nil) {
			return
		}
		var cursor int64
		for {
			page, err := l.Entries(ctx, s, auditlog.EntriesInput{Filter: f, Cursor: cursor, Limit: exportPageSize})
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(lo.Map(page.Items, func(e *ent.AuditEntry, _ int) []string { return exportRow(e) }), nil) || page.Next == 0 {
				return
			}
			cursor = page.Next
		}
	})
}

func exportRow(e *ent.AuditEntry) []string {
	v := auditlog.ViewOf(e)
	diff := ""
	if len(e.Diff) > 0 {
		if b, err := json.Marshal(e.Diff); err == nil {
			diff = string(b)
		}
	}
	return []string{
		strconv.FormatInt(v.ID, 10), v.OccurredAt.UTC().Format(time.RFC3339), v.ActorKind, lo.FromPtr(v.ActorID), lo.FromPtr(v.ActorName),
		v.Action, v.TargetType, lo.FromPtr(v.TargetID), lo.FromPtr(v.TargetName), diff,
		lo.FromPtr(v.RequestID), lo.FromPtr(v.IP), lo.FromPtr(v.UserAgent),
	}
}
