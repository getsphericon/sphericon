package automations

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationoccurrencecursor"
	"github.com/getsphericon/sphericon/ent/schema"
	"github.com/getsphericon/sphericon/internal/infra/db"
)

// The date scheduler (ADR 0126): the occurrences of an Automation's date Trigger are decided by
// its cursor, which the Publish of a date Trigger resets (resetCursor) and each tick moves
// (DecideOccurrences).

// The unique constraint of the cursor (ADR 0106): no client can break it.
var _ = db.Declare(
	db.Fault(schema.OccurrenceCursorIndex,
		"an Automation's cursor is written by an upsert on this index: a Publish resets it, a tick moves it"),
	db.Fault("automation_occurrence_cursors_automation_id_key",
		"the Automation's one cursor, the same key as the Workspace's index, which the upsert writes on"),
)

// resetCursor starts the active span of the Automation's date Trigger at, and stands its watermark
// there: nothing before it is ever decided. The Workspace zone and the Contacts seen are forgotten,
// so the next tick reads the Workspace's every Contact over the span it decides.
func resetCursor(ctx context.Context, s *ent.Scoped, automationID int64, at time.Time) error {
	return s.AutomationOccurrenceCursor().Create().
		SetAutomationID(automationID).
		SetSpanStart(at).
		SetDecidedThrough(at).
		OnConflictColumns(automationoccurrencecursor.FieldWorkspaceID, automationoccurrencecursor.FieldAutomationID).
		Update(func(u *ent.AutomationOccurrenceCursorScopedUpsert) {
			u.SetSpanStart(at).SetDecidedThrough(at).ClearWorkspaceZone().ClearContactsSeenThrough()
		}).
		Exec(ctx)
}
