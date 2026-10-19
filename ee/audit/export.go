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

// ExportCSV streams the scoped Workspace's Audit log entries matching the filter, newest first, to w as CSV: a
// header row, then one row per entry. It pages through Entries, so a long log never
// sits in memory, and it shows a Staff member as "Sphericon staff" like every read surface.
// The caller has already checked Licensed and the reader's role.
func (l *Log) ExportCSV(ctx context.Context, s *ent.Scoped, f events.AuditFilter, w io.Writer) error {
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
