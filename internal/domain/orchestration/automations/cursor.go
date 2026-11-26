package automations

import (
	"context"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationoccurrencecursor"
	"github.com/getsphericon/sphericon/ent/schema"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// The date scheduler's cursor of an Automation (ADR 0126) is a machine (ADR 0109): its position is
// the start of the active span and the watermark the occurrences are decided through, and both its
// moves, a tick that decides up to now and a Publish of a date Trigger that starts the span again,
// are one compare-and-swap of that position. A tick a Publish overtook moves nothing.

// cursorAt is where a cursor stands: its span's start and its watermark, in whole microseconds, as
// the database keeps them.
type cursorAt struct {
	spanStart, decidedThrough int64
}

// cursorPosition is the position of a span starting at start and decided through through.
func cursorPosition(start, through time.Time) cursorAt {
	return cursorAt{spanStart: start.UnixMicro(), decidedThrough: through.UnixMicro()}
}

// positionOf is where the cursor stands.
func positionOf(c *ent.AutomationOccurrenceCursor) cursorAt {
	return cursorPosition(c.SpanStart, c.DecidedThrough)
}

// The moves of a cursor.
const (
	// cursorTick decides the occurrences up to now and moves the watermark there.
	cursorTick = "tick"
	// cursorReset starts the span at a Publish and stands the watermark there.
	cursorReset = "reset"
)

// observed is what a move of a cursor saw: the Workspace time zone it read the occurrences in, and
// the latest change of a Contact it read; both nil for a reset, which forgets them.
type observed struct {
	workspaceZone *string
	contactsSeen  *time.Time
}

// moveCursor moves the cursor id from where it was read to to, keeping what the move observed. A
// cursor that moved since it was read is lifecycle.ErrInvalidTransition.
func moveCursor(ctx context.Context, s *ent.Scoped, id int64, move string, from, to cursorAt, seen observed) error {
	if from == to {
		return nil
	}
	machine := lifecycle.New(lifecycle.Storage[cursorAt]{
		Read: func(context.Context) (cursorAt, error) { return from, nil },
		Write: func(ctx context.Context, from, to cursorAt) (bool, error) {
			upd := s.AutomationOccurrenceCursor().Update().
				Where(
					automationoccurrencecursor.ID(id),
					automationoccurrencecursor.SpanStart(time.UnixMicro(from.spanStart)),
					automationoccurrencecursor.DecidedThrough(time.UnixMicro(from.decidedThrough)),
				).
				SetSpanStart(time.UnixMicro(to.spanStart)).
				SetDecidedThrough(time.UnixMicro(to.decidedThrough))
			if seen.workspaceZone == nil {
				upd.ClearWorkspaceZone()
			} else {
				upd.SetWorkspaceZone(*seen.workspaceZone)
			}
			if seen.contactsSeen == nil {
				upd.ClearContactsSeenThrough()
			} else {
				upd.SetContactsSeenThrough(*seen.contactsSeen)
			}
			n, err := upd.Save(ctx)
			return n == 1, err
		},
	})
	machine.Configure(from).Permit(move, to)
	return machine.FireCtx(ctx, move)
}

// The unique constraint of the cursor (ADR 0106): no client can break it.
var _ = db.Declare(
	db.Fault(schema.OccurrenceCursorIndex,
		"an Automation's cursor is created with ON CONFLICT DO NOTHING on this index, then moved by its machine"),
	db.Fault("automation_occurrence_cursors_automation_id_key",
		"the Automation's one cursor, the same key as the Workspace's index"),
)

// resetCursor starts the active span of the Automation's date Trigger at, and stands its watermark
// there: nothing before it is ever decided. The Workspace zone and the Contacts seen are forgotten,
// so the next tick reads the Workspace's every Contact over the span it decides. A cursor the
// Automation has none of yet is made there; a tick that moves it meanwhile is read again.
func resetCursor(ctx context.Context, s *ent.Scoped, automationID int64, now time.Time) error {
	to := cursorPosition(now, now)
	start := time.UnixMicro(to.spanStart)
	_, err := s.AutomationOccurrenceCursor().Create().
		SetAutomationID(automationID).
		SetSpanStart(start).
		SetDecidedThrough(start).
		OnConflictColumns(automationoccurrencecursor.FieldWorkspaceID, automationoccurrencecursor.FieldAutomationID).
		DoNothing().
		ID(ctx)
	made, err := db.Inserted(err)
	if err != nil || made {
		return err
	}
	for {
		cur, err := s.AutomationOccurrenceCursor().Query().Where(automationoccurrencecursor.AutomationID(automationID)).Only(ctx)
		if err != nil {
			return err
		}
		err = moveCursor(ctx, s, cur.ID, cursorReset, positionOf(cur), to, observed{})
		if !errors.Is(err, lifecycle.ErrInvalidTransition) {
			return err
		}
	}
}
