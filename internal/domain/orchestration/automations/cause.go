package automations

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationskippedentry"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
)

// cause is what caused an entry, which enrolls a Contact in an Automation once (ADR 0127): the
// Event whose Trigger it is, by its source id, or the Rule Firing whose Enroll Action it is. An
// entry has one or neither: an Enrollment no Event or Firing caused (a Firing run without its
// stored Event's source id) is no cause's, and nothing makes it once only.
type cause struct {
	sourceID string
	firingID int64
}

// byEvent is whether the cause is the Event of a Trigger.
func (c cause) byEvent() bool { return c.sourceID != "" }

// byFiring is whether the cause is a Rule Firing.
func (c cause) byFiring() bool { return !c.byEvent() && c.firingID != 0 }

// made is whether the cause made the Enrollment; a cause of neither made none.
func (c cause) made(run *ent.AutomationRun) bool {
	switch {
	case c.byEvent():
		return lo.FromPtr(run.TriggerSourceID) == c.sourceID
	case c.byFiring():
		return lo.FromPtr(run.FiringID) == c.firingID
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
	}
	run, err := create.Save(ctx)
	if err != nil {
		return 0, err
	}
	return run.ID, nil
}

// insertSkip writes the skipped entry create builds, of the cause: once per Automation and Event
// (or Firing), so an entry handled again records nothing new.
func (c cause) insertSkip(ctx context.Context, create *ent.AutomationSkippedEntryScopedCreate) error {
	switch {
	case c.byEvent():
		return create.SetSourceID(c.sourceID).
			OnConflictColumns(automationskippedentry.FieldWorkspaceID, automationskippedentry.FieldAutomationID, automationskippedentry.FieldSourceID).
			DoNothing().Exec(ctx)
	case c.byFiring():
		return create.SetFiringID(c.firingID).
			OnConflictColumns(automationskippedentry.FieldWorkspaceID, automationskippedentry.FieldAutomationID, automationskippedentry.FieldFiringID).
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
