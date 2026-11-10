package joindomains

import (
	"context"
	"time"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/joindomain"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// state is a Join domain's verification, kept as verified_at (ADR 0109). It is a machine, not a
// latch: a domain whose record is gone, or that another Workspace takes over, is pending again. The
// states and triggers are the module's own types: the state is derived from a moment, so it has no
// ent enum (ADR 0046).
type state string

const (
	pending  state = "pending"
	verified state = "verified"
)

// trigger is what a check of a TXT record found.
type trigger string

const (
	// prove: the record is published and no other Workspace holds the domain.
	prove trigger = "prove"
	// lose: the record is gone, so a re-check reverts the domain or another Workspace takes it over.
	lose trigger = "lose"
)

// verification is the machine of row's verification as row was read (ADR 0046, ADR 0109). Its
// write is a compare-and-swap on the verified_at read, not only on the state: a check that read one
// verification never clears another made since (the domain handed over and verified again), and a
// stale move is lifecycle.ErrInvalidTransition. A move stamps at as the check time, and at is the
// new verified_at too. ts is the scope of the transaction the move is written in.
func verification(ts *ent.Scoped, row *ent.JoinDomain, at time.Time) *stateless.StateMachine {
	m := lifecycle.New(lifecycle.Storage[state]{
		Read: func(context.Context) (state, error) { return stateOf(row), nil },
		Write: func(ctx context.Context, _, to state) (bool, error) {
			upd := ts.JoinDomain().UpdateOneID(row.ID).SetLastCheckedAt(at)
			if row.VerifiedAt == nil {
				upd.Where(joindomain.VerifiedAtIsNil())
			} else {
				upd.Where(joindomain.VerifiedAt(*row.VerifiedAt))
			}
			if to == verified {
				upd.SetVerifiedAt(at)
			} else {
				upd.ClearVerifiedAt()
			}
			err := upd.Exec(ctx)
			if ent.IsNotFound(err) {
				return false, nil
			}
			return err == nil, err
		},
	})
	m.Configure(pending).Permit(prove, verified)
	m.Configure(verified).Permit(lose, pending)
	return m
}

// stateOf is the state of the verification row holds.
func stateOf(row *ent.JoinDomain) state {
	if Verified(row) {
		return verified
	}
	return pending
}
