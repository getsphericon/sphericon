// Package instanceaudit owns the Instance audit log (ADR 0070): the record of the changes a Staff
// member makes to the Instance's own settings, who made each and the old and the new values. It is
// the Instance's counterpart of a Workspace's Audit log, and it is core: written on every Instance,
// whatever the License key. Nothing in it belongs to a Workspace, so it holds the raw ent client,
// and Workspace deletion never touches it.
//
// A change is recorded with Record, on the transaction client of the change itself, so the setting
// and its entry commit together or not at all. Each Instance-level Staff change names its action
// here.
package instanceaudit

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/instanceauditentry"
)

// ActionDefaultSendCapChange records a change of the default Send cap of new Workspaces
// (ADR 0067).
const ActionDefaultSendCapChange = "instance.default_send_cap_change"

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

// Entries is a page of the log, newest first, each with the Staff member who made it: at most
// limit entries older than the entry cursor (0 starts from the newest), and the cursor of the next
// page (0 on the last page).
func Entries(ctx context.Context, client *ent.Client, cursor int64, limit int) ([]*ent.InstanceAuditEntry, int64, error) {
	q := client.InstanceAuditEntry.Query().
		WithStaffMember().
		Order(ent.Desc(instanceauditentry.FieldID)).
		Limit(limit + 1)
	if cursor > 0 {
		q = q.Where(instanceauditentry.IDLT(cursor))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("read the instance audit log: %w", err)
	}
	if len(rows) <= limit {
		return rows, 0, nil
	}
	rows = rows[:limit]
	return rows, rows[limit-1].ID, nil
}
