// Package rulesunchecked holds the cancellation of the queued Rule evaluations that name a Contact.
// It is a step of a Contact's Erasure (ADR 0021), which harms a live Contact when it runs alone:
// the use case internal/usecase/contacterasure runs each owner's step in its order, in the one
// transaction of the Erasure; only a use case may import this one (ADR 0110), and each that does
// builds its Module. The rules module does not import it.
package rulesunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// Module is the rules module's unchecked operations (docs/architecture/modules.md): the
// cancellation of the queued evaluations that name a Contact.
type Module struct {
	bus *events.Bus
}

// New builds the unchecked operations over the bus whose Erasure transaction purges the queued
// jobs.
func New(bus *events.Bus) *Module {
	return &Module{bus: bus}
}

// CancelPending cancels the queued evaluations that name a Contact (Erasure, ADR 0021): the job
// carries the triggering Event's properties. An Actions job carries only a Firing id and finds
// the Firing gone once the Contact row is deleted, which cascades to its Firings. It runs inside
// the Erasure's transaction, which s is bound to (events.Bus.WithinScopedPurgeTx).
func (m *Module) CancelPending(ctx context.Context, s *ent.Scoped, contactID int64) error {
	return m.bus.PurgeJobsIn(ctx, s, jobkind.RuleEvaluate, []int64{contactID})
}
