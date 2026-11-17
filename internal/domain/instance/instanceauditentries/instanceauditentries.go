// Package instanceauditentries owns the Instance audit log (ADR 0070): the record of the changes a Staff
// member makes to the Instance's own settings, who made each and the old and the new values. It is
// the Instance's counterpart of a Workspace's Audit log, and it is core: written on every Instance,
// whatever the License key. Nothing in it belongs to a Workspace, so it holds the raw ent client,
// and Workspace deletion never touches it.
//
// A change is recorded with Log.Record, on the transaction client of the change itself, so the
// setting and its entry commit together or not at all. Each Instance-level Staff change names its action
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
// a module below the Instance names and records through Log.RecordSignupItem handed to it (the
// Sign-up items).
var Actions = append([]string{ActionDefaultSendCapChange}, signupitems.AuditActions...)

// Log is the Instance audit log, read over the raw client.
type Log struct {
	client *ent.Client
}

// New builds the log over the raw client it reads the entries through.
func New(client *ent.Client) *Log { return &Log{client: client} }

// RecordInput is one entry of the log: the Staff member StaffMemberID made the change Action, with
// Diff the old and the new value of each changed field (events.Change, events.ChangeOf).
type RecordInput struct {
	StaffMemberID int64
	Action        string
	Diff          map[string]any
}

// Record writes the entry in; the log sets the rest. tx is the transaction client of the change, so
// a rolled-back change leaves no entry.
func (l *Log) Record(ctx context.Context, tx *ent.Client, in RecordInput) error {
	err := tx.InstanceAuditEntry.Create().
		SetStaffMemberID(in.StaffMemberID).
		SetAction(in.Action).
		SetDiff(in.Diff).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("record %s: %w", in.Action, err)
	}
	return nil
}

// RecordSignupItem is Record of a change of the Sign-up items (signupitems.AuditRecorder): that
// module sits below the log and names the entry's fields with a type of its own.
func (l *Log) RecordSignupItem(ctx context.Context, tx *ent.Client, entry signupitems.AuditEntry) error {
	return l.Record(ctx, tx, RecordInput(entry))
}

// Page is one page of the log, newest first, and the cursor of the next page ("" on the last).
type Page struct {
	Entries []*ent.InstanceAuditEntry
	Next    string
}

// Entries is a page of the log, newest first, each with the Staff member who made it: at most
// limit entries older than the one cursor names ("" starts from the newest). A cursor is the
// Audit log's (auditlog.Cursor): one the log never handed out is auditlog.ErrBadCursor.
func (l *Log) Entries(ctx context.Context, cursor string, limit int) (Page, error) {
	after, err := auditlog.Cursor(cursor)
	if err != nil {
		return Page{}, err
	}
	q := l.client.InstanceAuditEntry.Query().
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
