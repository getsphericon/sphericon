package automations

import (
	"context"

	"entgo.io/ent/dialect/sql"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationskippedentry"
	"github.com/getsphericon/sphericon/ent/schema"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
)

// cause is what caused an entry, which enrolls a Contact in an Automation once (ADR 0127): the
// Event whose Trigger it is, by its source id (byEvent), the Rule Firing whose Enroll Action it is
// (byFiring), or an occurrence of a date Trigger (byOccurrence, ADR 0126). An Enrollment no Event,
// Firing or occurrence caused (a Firing run without its stored Event's source id) is noCause's, and
// nothing makes it once only. Each kind writes its Enrollment and its skipped entry on its own
// unique index.
//
//sumtype:decl
type cause interface {
	// made is whether the cause made the Enrollment.
	made(run *ent.AutomationRun) bool
	// insertRun writes the Enrollment create builds, made by the cause: an Enrollment the cause made
	// already is the answer and nothing is written (db.Inserted reads the conflict as no row).
	insertRun(ctx context.Context, create *ent.AutomationRunScopedCreate) (int64, error)
	// insertSkip writes the skipped entry create builds, of the cause, for the reason: an entry
	// handled again records nothing new.
	insertSkip(ctx context.Context, create *ent.AutomationSkippedEntryScopedCreate, reason automationskippedentry.Reason) error
}

// byEvent is the Event of a Trigger, by its source id: it enrolls once per Automation.
type byEvent struct{ sourceID string }

// byFiring is a Rule Firing: it enrolls once per Automation.
type byFiring struct{ firingID int64 }

// byOccurrence is an occurrence of a date Trigger: it enrolls once per Automation and Contact, and
// records one deciding skipped entry and one `send_locked` one (schema.OccurrenceSkipWhere).
type byOccurrence struct{ occurrence datetrigger.Occurrence }

// noCause is an entry nothing makes once only.
type noCause struct{}

// eventCause is the cause of an entry on the Event of the source id, none without one.
func eventCause(sourceID string) cause {
	if sourceID == "" {
		return noCause{}
	}
	return byEvent{sourceID: sourceID}
}

// firingCause is the cause of an entry on the Firing of the id, none without one.
func firingCause(firingID int64) cause {
	if firingID == 0 {
		return noCause{}
	}
	return byFiring{firingID: firingID}
}

// sourceOf is the source id of the Event that caused the entry, empty for any other cause.
func sourceOf(c cause) string {
	e, _ := c.(byEvent)
	return e.sourceID
}

func (c byEvent) made(run *ent.AutomationRun) bool {
	return lo.FromPtr(run.TriggerSourceID) == c.sourceID
}

func (c byFiring) made(run *ent.AutomationRun) bool { return lo.FromPtr(run.FiringID) == c.firingID }

func (c byOccurrence) made(run *ent.AutomationRun) bool {
	return run.Occurrence != nil && run.Occurrence.Occurrence() == c.occurrence
}

func (noCause) made(*ent.AutomationRun) bool { return false }

func (c byEvent) insertRun(ctx context.Context, create *ent.AutomationRunScopedCreate) (int64, error) {
	return create.SetTriggerSourceID(c.sourceID).
		OnConflictColumns(automationrun.FieldWorkspaceID, automationrun.FieldAutomationID, automationrun.FieldTriggerSourceID).
		DoNothing().ID(ctx)
}

func (c byFiring) insertRun(ctx context.Context, create *ent.AutomationRunScopedCreate) (int64, error) {
	return create.SetFiringID(c.firingID).
		OnConflictColumns(automationrun.FieldWorkspaceID, automationrun.FieldAutomationID, automationrun.FieldFiringID).
		DoNothing().ID(ctx)
}

func (c byOccurrence) insertRun(ctx context.Context, create *ent.AutomationRunScopedCreate) (int64, error) {
	return create.SetOccurrence(datetrigger.Stored(c.occurrence)).
		OnConflictColumnsWhere(sql.NotNull(automationrun.FieldOccurrence),
			automationrun.FieldWorkspaceID, automationrun.FieldAutomationID, automationrun.FieldContactID, automationrun.FieldOccurrence).
		DoNothing().ID(ctx)
}

func (noCause) insertRun(ctx context.Context, create *ent.AutomationRunScopedCreate) (int64, error) {
	run, err := create.Save(ctx)
	if err != nil {
		return 0, err
	}
	return run.ID, nil
}

func (c byEvent) insertSkip(ctx context.Context, create *ent.AutomationSkippedEntryScopedCreate, _ automationskippedentry.Reason) error {
	return create.SetSourceID(c.sourceID).
		OnConflictColumns(automationskippedentry.FieldWorkspaceID, automationskippedentry.FieldAutomationID, automationskippedentry.FieldSourceID).
		DoNothing().Exec(ctx)
}

func (c byFiring) insertSkip(ctx context.Context, create *ent.AutomationSkippedEntryScopedCreate, _ automationskippedentry.Reason) error {
	return create.SetFiringID(c.firingID).
		OnConflictColumns(automationskippedentry.FieldWorkspaceID, automationskippedentry.FieldAutomationID, automationskippedentry.FieldFiringID).
		DoNothing().Exec(ctx)
}

func (c byOccurrence) insertSkip(ctx context.Context, create *ent.AutomationSkippedEntryScopedCreate, reason automationskippedentry.Reason) error {
	return create.SetOccurrence(datetrigger.Stored(c.occurrence)).
		OnConflictColumnsWhere(occurrenceSkipIndexOf(reason),
			automationskippedentry.FieldWorkspaceID, automationskippedentry.FieldAutomationID,
			automationskippedentry.FieldContactID, automationskippedentry.FieldOccurrence).
		DoNothing().Exec(ctx)
}

func (noCause) insertSkip(ctx context.Context, create *ent.AutomationSkippedEntryScopedCreate, _ automationskippedentry.Reason) error {
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
// occurrence for the reason is written on, as the schema declares it: the one of `send_locked`, or
// the one of the reasons that decide the occurrence.
func occurrenceSkipIndexOf(reason automationskippedentry.Reason) *sql.Predicate {
	where := schema.OccurrenceSkipWhere
	if reason == automationskippedentry.ReasonSendLocked {
		where = schema.OccurrenceSendLockedWhere
	}
	// raw sql: Postgres infers a partial index from the ON CONFLICT predicate only when that
	// predicate implies the index's, which a bound argument never does once the statement runs on a
	// generic plan; the predicate is the index's own, from the schema.
	return sql.P(func(b *sql.Builder) { b.WriteString(where) })
}
