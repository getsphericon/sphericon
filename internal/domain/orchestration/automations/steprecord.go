package automations

import (
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
)

// A Step record is what an Enrollment keeps of one Step it reached (ADR 0124). The move that puts
// the Enrollment on a Step writes its record (reach), and the move that takes it off stamps it
// (leave), each in the transaction of that move and after the compare-and-set on the
// Enrollment's cursor, so a move that lost a race writes no record.

var _ = db.Declare(
	db.Fault("automation_step_records_workspace_id_run_id_step_id",
		"a Step record is written by the move that puts the Enrollment on the Step, which its cursor's compare-and-set lets happen once, and a version's Steps have no cycles"),
)

// dueOf is when the Step st is due for an Enrollment that reaches it at now: a wait's delay after
// now, computed once, when it is reached (ADR 0124); nil for any other Step, which runs at once.
func dueOf(st *automationstep.Step, now time.Time) *time.Time {
	w, ok := st.Payload.(automationstep.Wait)
	if !ok {
		return nil
	}
	due := now.Add(time.Duration(w.Seconds) * time.Second)
	return &due
}

// reach records that the Enrollment runID reached the Step of the id at now, due at due (a wait's
// due moment, nil for any other Step).
func reach(ctx context.Context, s *ent.Scoped, runID int64, stepID string, now time.Time, due *time.Time) error {
	return s.AutomationStepRecord().Create().
		SetRunID(runID).
		SetStepID(stepID).
		SetReachedAt(now).
		SetNillableDueAt(due).
		Exec(ctx)
}

// leave records that the Enrollment runID left the Step of the id at now: skipped for why when
// why is set, done otherwise.
func leave(ctx context.Context, s *ent.Scoped, runID int64, stepID string, now time.Time, why skipreason.Reason) error {
	u := s.AutomationStepRecord().Update().
		Where(
			automationsteprecord.RunID(runID),
			automationsteprecord.StepID(stepID),
			automationsteprecord.LeftAtIsNil(),
		).
		SetLeftAt(now).
		SetOutcome(automationsteprecord.OutcomeDone)
	if why != "" {
		u.SetOutcome(automationsteprecord.OutcomeSkipped).SetSkipReason(why)
	}
	n, err := u.Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("leave step %s of run %d: the Enrollment has no record of reaching it", stepID, runID)
	}
	return nil
}
