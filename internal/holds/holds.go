// Package holds reads Staff holds (ADR 0009): the reversible freezes a service sets
// on a Workspace's outbound sending. It is the one place that decides a hold is active,
// so the send path, the site API and the console agree: a hold is active while its
// expiry is after the clock, and a lapsed one is ignored at once, with no cleanup job.
//
// Setting and clearing live in ee/staff (the service endpoints); this package only
// reads, through the scoped client, and holds no raw client.
package holds

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	enthold "github.com/getsphericon/sphericon/ent/hold"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/outbound"
)

// The reason codes that mark a hold as a billing hold (ADR 0009): sending is paused for
// a money reason. Nothing else about the hold differs; the code is the whole marker.
const (
	ReasonNonPayment  = "non_payment"
	ReasonFreePlanCap = "free_plan_cap"
)

// IsBilling reports whether a hold's reason code marks it a billing hold. Every surface
// that tells a billing hold apart (the owner banner, the Audit log, the console) asks here.
func IsBilling(reasonCode string) bool {
	return reasonCode == ReasonNonPayment || reasonCode == ReasonFreePlanCap
}

// ActiveAt is the one rule for an active hold: its expiry is after now. Every read
// goes through it, in SQL here and in Go in Live.
func ActiveAt(now time.Time) predicate.Hold { return enthold.ExpiresAtGT(now) }

// Active lists the holds of the scope's Workspace that are active at now, ordered by
// key. It is what the owner-facing and console reads of a Workspace's holds use.
func Active(ctx context.Context, s *ent.Scoped, now time.Time) ([]*ent.Hold, error) {
	return s.Hold().Query().
		Where(ActiveAt(now)).
		Order(enthold.ByKey()).
		All(ctx)
}

// Live keeps the holds of rows that are active at now, in their order. It is Active
// over rows already loaded (the eager-loaded edge, where no predicate can be passed), so
// it states ActiveAt's rule in Go: keep it in step.
func Live(rows []*ent.Hold, now time.Time) []*ent.Hold {
	return lo.Filter(rows, func(h *ent.Hold, _ int) bool { return h.ExpiresAt.After(now) })
}

// Freezer answers the send path's "may this Workspace send now" from the active holds
// (outbound.Freezer): any one active hold freezes sending, and a second keeps it frozen
// after the first is cleared.
type Freezer struct {
	// Now is the clock; nil is time.Now.
	Now func() time.Time
	// Enabled reports whether holds are honoured at all (the `operator` license); nil
	// is always.
	Enabled func() bool
}

var _ outbound.Freezer = Freezer{}

// Frozen implements outbound.Freezer.
func (f Freezer) Frozen(ctx context.Context, s *ent.Scoped, _ *ent.Workspace) (string, error) {
	if f.Enabled != nil && !f.Enabled() {
		return "", nil
	}
	now := time.Now
	if f.Now != nil {
		now = f.Now
	}
	held, err := s.Hold().Query().Where(ActiveAt(now())).Exist(ctx)
	if err != nil || !held {
		return "", err
	}
	return outbound.HoldStaff, nil
}
