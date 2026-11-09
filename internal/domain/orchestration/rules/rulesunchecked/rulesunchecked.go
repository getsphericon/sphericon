// Package rulesunchecked holds the cancellation of the queued Rule evaluations that name a Contact.
// It is a step of a Contact's Erasure (ADR 0021), which harms a live Contact when it runs alone:
// the use case internal/usecase/contacterasure runs each owner's step in its order, in the one
// transaction of the Erasure; only a use case may import this one (ADR 0110). The rules module does
// not import it.
package rulesunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// CancelPending cancels the queued evaluations that name a Contact (Erasure, ADR 0021): the job
// carries the triggering Event's properties. An Actions job carries only a Firing id and finds
// the Firing gone once the Contact row is deleted, which cascades to its Firings. It runs inside
// the caller's transaction.
func CancelPending(ctx context.Context, _ *ent.Scoped, contactID int64, purger events.QueuePurger) error {
	return purger.PurgeJobs(ctx, jobkind.RuleEvaluate, []int64{contactID})
}
