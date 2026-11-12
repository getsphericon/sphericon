// Package instanceauditentries owns the Instance audit log (ADR 0070): the record of the changes a Staff
// member makes to the Instance's own settings, who made each and the old and the new values. It is
// the Instance's counterpart of a Workspace's Audit log, and it is core: written on every Instance,
// whatever the License key. Nothing in it belongs to a Workspace, so it holds the raw ent client,
// and Workspace deletion never touches it.
//
// A change is recorded with Record, on the transaction client of the change itself, so the setting
// and its entry commit together or not at all. Each Instance-level Staff change names its action
// here.
package instanceauditentries

import (
	"context"
	"fmt"
	"github.com/getsphericon/sphericon/internal/domain/identity/signupitems"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/instanceauditentry"
	"github.com/getsphericon/sphericon/internal/domain/insights/auditlog"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// ActionDefaultSendCapChange records a change of the default Send cap of new Workspaces
// (ADR 0067).
const ActionDefaultSendCapChange = "instance.default_send_cap_change"

// Actions is every action the log records, which the Staff contract declares: its own, and those
// a module below the Instance names and records through Record handed to it (the Sign-up items).
var Actions = append([]string{ActionDefaultSendCapChange}, signupitems.AuditActions...)

// Record writes one entry: the Staff member staffMemberID made the change action, with diff the
// old and the new value of each changed field (events.Change, events.ChangeOf). tx is the transaction client of the
// change, so a rolled-back change leaves no entry.
func Record(ctx context.Context, tx *ent.Client, staffMemberID int64, action string, diff map[string]any) error {
	err := tx.InstanceAuditEntry.Create().
		SetStaffMemberID(staffMemberID).
		SetAction(action).
		SetDiff(diff).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("record %s: %w", action, err)
	}
	return nil
}

// Page is one page of the log, newest first, and the cursor of the next page ("" on the last).
type Page struct {
	Entries []*ent.InstanceAuditEntry
	Next    string
}

// Entries is a page of the log, newest first, each with the Staff member who made it: at most
// limit entries older than the one cursor names ("" starts from the newest). A cursor is the
// Audit log's (auditlog.Cursor): one the log never handed out is auditlog.ErrBadCursor.
func Entries(ctx context.Context, client *ent.Client, cursor string, limit int) (Page, error) {
	after, err := auditlog.Cursor(cursor)
	if err != nil {
		return Page{}, err
	}
	q := client.InstanceAuditEntry.Query().
		WithStaffMember().
		Order(ent.Desc(instanceauditentry.FieldID)).
		Limit(limit + 1)
	if after > 0 {
		q = q.Where(instanceauditentry.IDLT(after))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return Page{}, fmt.Errorf("read the instance audit log: %w", err)
	}
	page := pagination.ByID(rows, limit, func(e *ent.InstanceAuditEntry) int64 { return e.ID })
	out := Page{Entries: page.Items}
	if page.Next > 0 {
		out.Next = auditlog.NextCursor(page.Next)
	}
	return out, nil
}
