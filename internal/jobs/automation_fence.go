package jobs

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/lifecycle"
)

// Fencer runs a step's side effect (a mail, an HTTP call) so that it is ordered against an
// exit of the Enrollment: automations.Stale(err) when the Enrollment was no longer active
// when the effect would have started (then effect did not run), otherwise effect's own error.
type Fencer func(ctx context.Context, s *ent.Scoped, runID int64, effect func(context.Context) error) error

// FenceOn is the Fencer of a database with real connections (ADR 0046): effect runs while a
// share lock is held on the Enrollment's row, taken only if the row is still `active`. Every
// move out of `active` is an UPDATE of that row, and an UPDATE waits for a share lock, so
//
//   - an exit that arrives while the effect is in flight returns only after it finished: the
//     effect is ordered before the exit, never after it;
//   - an exit committed before the lock is seen under it (the lock's WHERE is re-checked once a
//     concurrent update commits), so the effect never starts.
//
// The lock lives on a transaction of its own that does nothing else, and effect runs on the
// pool's other connections as it always did: the step's compare-and-swap that follows (Advance,
// Complete, Fail) is made after the lock is released, so it cannot wait on itself, and the
// outbound claim, lease and idempotency key are untouched. The hold lasts as long as effect
// runs, which the job's context, the provider's and the HTTP client's timeouts bound; the
// transaction ends when effect returns or ctx is done. Only the lock is read-only, so there is
// nothing to commit.
//
// The lock needs one pool connection beyond the effect's own for the effect's duration.
func FenceOn(client *ent.Client) Fencer {
	return func(ctx context.Context, s *ent.Scoped, runID int64, effect func(context.Context) error) error {
		tx, err := client.Tx(ctx)
		if err != nil {
			return fmt.Errorf("fence run %d: %w", runID, err)
		}
		defer func() { _ = tx.Rollback() }()

		locked, err := s.InTx(tx.Client(), nil).AutomationRun().Query().
			Where(automationrun.ID(runID), automationrun.StatusEQ(automationrun.StatusActive)).
			ForShare().
			IDs(ctx)
		if err != nil {
			return fmt.Errorf("fence run %d: %w", runID, err)
		}
		if len(locked) == 0 {
			return lifecycle.ErrInvalidTransition
		}
		return effect(ctx)
	}
}
