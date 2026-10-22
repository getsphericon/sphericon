package rules

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/jobkind"
)

// CancelPending cancels the queued evaluations that name a Contact (Erasure, ADR 0021): the job
// carries the triggering Event's properties. An Actions job carries only a Firing id and finds
// the Firing gone once EraseFirings has deleted it. It runs inside the caller's transaction.
func CancelPending(ctx context.Context, _ *ent.Scoped, contactID int64, purger events.QueuePurger) error {
	return purger.PurgeJobs(ctx, jobkind.RuleEvaluate, []int64{contactID})
}

// EraseFirings deletes a Contact's Firings (Erasure, ADR 0021): they name the Contact and the
// Event that fired the Rule.
func EraseFirings(ctx context.Context, s *ent.Scoped, contactID int64) error {
	_, err := s.RuleFiring().Delete().Where(rulefiring.ContactID(contactID)).Exec(ctx)
	return err
}
