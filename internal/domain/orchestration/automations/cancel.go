package automations

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// CancelPending cancels a Contact's pending Automation work (Erasure, ADR 0021): the
// queued step and HTTP-step jobs of the Contact's runs and the queued trigger evaluations
// that name the Contact. It reads the ids of the runs before the Contact row goes, since the runs
// (a failure's response excerpt sits on one) are deleted with it by their cascading foreign key.
// It runs inside the caller's transaction (s and purger come from the same one); a job that is
// already running is not purged.
func CancelPending(ctx context.Context, s *ent.Scoped, contactID int64, purger events.QueuePurger) error {
	runIDs, err := s.AutomationRun().Query().Where(automationrun.ContactID(contactID)).IDs(ctx)
	if err != nil {
		return err
	}
	if err := purger.PurgeJobs(ctx, jobkind.EvaluateTrigger, []int64{contactID}); err != nil {
		return err
	}
	if err := purger.PurgeJobs(ctx, jobkind.RunStep, runIDs); err != nil {
		return err
	}
	return purger.PurgeJobs(ctx, jobkind.HTTPStep, runIDs)
}
