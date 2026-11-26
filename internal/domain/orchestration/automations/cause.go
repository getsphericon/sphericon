package automations

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationskippedentry"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
)

// cause is what caused an entry, which enrolls a Contact in an Automation once (ADR 0127): the
// Event whose Trigger it is, by its source id, the Rule Firing whose Enroll Action it is, or the
// occurrence of a date Trigger (ADR 0126), a datetrigger.Occurrence. An
// entry has one or none: an Enrollment no Event, Firing or occurrence caused (a Firing run without
// its stored Event's source id) is no cause's, and nothing makes it once only.
type cause struct {
	sourceID   string
	firingID   int64
	occurrence datetrigger.Occurrence
}

// byEvent is whether the cause is the Event of a Trigger.
func (c cause) byEvent() bool { return c.sourceID != "" }

// byFiring is whether the cause is a Rule Firing.
func (c cause) byFiring() bool { return !c.byEvent() && c.firingID != 0 }

// byOccurrence is whether the cause is an occurrence of a date Trigger.
func (c cause) byOccurrence() bool { return c.occurrence != nil }

// made is whether the cause made the Enrollment; a cause of neither made none.
func (c cause) made(run *ent.AutomationRun) bool {
	switch {
	case c.byEvent():
		return lo.FromPtr(run.TriggerSourceID) == c.sourceID
	case c.byFiring():
		return lo.FromPtr(run.FiringID) == c.firingID
	case c.byOccurrence():
		return run.Occurrence != nil && run.Occurrence.Occurrence == c.occurrence
	}
	return false
}

// insertRun writes the Enrollment create builds, made by the cause: one Event, or one Firing,
// enrolls a Contact in an Automation once, so an Enrollment it made already is the answer and
// nothing is written (db.Inserted reads the conflict as no row).
func (c cause) insertRun(ctx context.Context, create *ent.AutomationRunScopedCreate) (int64, error) {
	switch {
	case c.byEvent():
		return create.SetTriggerSourceID(c.sourceID).
			OnConflictColumns(automationrun.FieldWorkspaceID, automationrun.FieldAutomationID, automationrun.FieldTriggerSourceID).
			DoNothing().ID(ctx)
	case c.byFiring():
		return create.SetFiringID(c.firingID).
			OnConflictColumns(automationrun.FieldWorkspaceID, automationrun.FieldAutomationID, automationrun.FieldFiringID).
			DoNothing().ID(ctx)
	case c.byOccurrence():
		return create.SetOccurrence(datetrigger.Stored(c.occurrence)).
			OnConflictColumnsWhere(sql.NotNull(automationrun.FieldOccurrence),
				automationrun.FieldWorkspaceID, automationrun.FieldAutomationID, automationrun.FieldContactID, automationrun.FieldOccurrence).
			DoNothing().ID(ctx)
	}
	run, err := create.Save(ctx)
	if err != nil {
		return 0, err
	}
	return run.ID, nil
}

// insertSkip writes the skipped entry create builds, of the cause, for the reason: once per
// Automation and Event (or Firing), and once per Automation, Contact and occurrence for a reason
// that decides it or for `send_locked`, which does not, so an entry handled again records nothing
// new.
func (c cause) insertSkip(ctx context.Context, create *ent.AutomationSkippedEntryScopedCreate, reason automationskippedentry.Reason) error {
	switch {
	case c.byEvent():
		return create.SetSourceID(c.sourceID).
			OnConflictColumns(automationskippedentry.FieldWorkspaceID, automationskippedentry.FieldAutomationID, automationskippedentry.FieldSourceID).
			DoNothing().Exec(ctx)
	case c.byFiring():
		return create.SetFiringID(c.firingID).
			OnConflictColumns(automationskippedentry.FieldWorkspaceID, automationskippedentry.FieldAutomationID, automationskippedentry.FieldFiringID).
			DoNothing().Exec(ctx)
	case c.byOccurrence():
		return create.SetOccurrence(datetrigger.Stored(c.occurrence)).
			OnConflictColumnsWhere(occurrenceSkipIndexOf(reason),
				automationskippedentry.FieldWorkspaceID, automationskippedentry.FieldAutomationID,
				automationskippedentry.FieldContactID, automationskippedentry.FieldOccurrence).
			DoNothing().Exec(ctx)
	}
	return create.Exec(ctx)
}

// begunBy reports whether the Event stored with sourceID is the one the Enrollment began on (ADR
// 0133): its Trigger's, or the one the Rule whose Firing made it fired on. That Event never ends
// it.
func begunBy(ctx context.Context, s *ent.Scoped, firings *rules.Firings, run *ent.AutomationRun, sourceID string) (bool, error) {
	if sourceID == "" {
		return false, nil
	}
	if run.TriggerSourceID != nil {
		return *run.TriggerSourceID == sourceID, nil
	}
	if run.FiringID == nil {
		return false, nil
	}
	fired, err := firings.SourceOf(ctx, s, *run.FiringID)
	if err != nil {
		return false, err
	}
	return fired == sourceID, nil
}

// occurrenceSkipIndexOf is the predicate of the partial unique index a skipped entry of an
// occurrence for the reason is written on: the one of `send_locked`, or the one of the reasons that
// decide the occurrence.
func occurrenceSkipIndexOf(reason automationskippedentry.Reason) *sql.Predicate {
	op := "<>"
	if reason == automationskippedentry.ReasonSendLocked {
		op = "="
	}
	// raw sql: Postgres infers a partial index from the ON CONFLICT predicate only when that
	// predicate implies the index's, which a bound argument never does once the statement runs on a
	// generic plan; the reason is a constant of the enum, written as the literal the index holds.
	return sql.P(func(b *sql.Builder) {
		b.WriteString(fmt.Sprintf("%s IS NOT NULL AND %s %s '%s'",
			automationskippedentry.FieldOccurrence, automationskippedentry.FieldReason, op, automationskippedentry.ReasonSendLocked))
	})
}
