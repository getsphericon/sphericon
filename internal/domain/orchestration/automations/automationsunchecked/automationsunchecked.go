// Package automationsunchecked holds the cancellation of a Contact's queued Automation work. It is
// a step of a Contact's Erasure (ADR 0021), which harms a live Contact when it runs alone: the use
// case internal/usecase/contacterasure runs each owner's step in its order, in the one transaction
// of the Erasure. It also holds the count message retention keeps of an Automation's deleted
// messages (ADR 0143), which internal/usecase/messageretention runs. Only a use case may import
// this one (ADR 0110), and each that does builds its Module. The automations module does not
// import it.
package automationsunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// Module is the automations module's unchecked operations (docs/architecture/modules.md): the
// cancellation of a Contact's queued Automation work and the count of an Automation's pruned
// messages.
type Module struct {
	bus *events.Bus
}

// New builds the unchecked operations over the bus whose Erasure transaction purges the queued
// jobs.
func New(bus *events.Bus) *Module {
	return &Module{bus: bus}
}

// CancelPending cancels a Contact's pending Automation work (Erasure, ADR 0021): the
// queued step and HTTP-step jobs of the Contact's runs and the queued trigger evaluations
// that name the Contact. It reads the ids of the runs before the Contact row goes, since the runs
// (a failure's response excerpt sits on one) are deleted with it by their cascading foreign key.
// It runs inside the Erasure's transaction, which s is bound to (events.Bus.WithinScopedPurgeTx),
// and purges the jobs in it; a job that is already running is not purged.
func (m *Module) CancelPending(ctx context.Context, s *ent.Scoped, contactID int64) error {
	runIDs, err := s.AutomationRun().Query().Where(automationrun.ContactID(contactID)).IDs(ctx)
	if err != nil {
		return err
	}
	if err := m.bus.PurgeJobsIn(ctx, s, jobkind.EvaluateTrigger, []int64{contactID}); err != nil {
		return err
	}
	if err := m.bus.PurgeJobsIn(ctx, s, jobkind.RunStep, runIDs); err != nil {
		return err
	}
	return m.bus.PurgeJobsIn(ctx, s, jobkind.HTTPStep, runIDs)
}
