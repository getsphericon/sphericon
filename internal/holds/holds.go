// Package holds reads Operator holds (ADR 0009): the reversible freezes a service sets
// on a Workspace's outbound sending. It is the one place that decides a hold is active,
// so the send path, the site API and the console agree: a hold is active while its
// expiry is after the clock, and a lapsed one is ignored at once, with no cleanup job.
//
// Setting and clearing live in ee/operator (the service endpoints); this package only
// reads, through the scoped client, and holds no raw client.
package holds

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	enthold "github.com/getsphericon/sphericon/ent/hold"
	"github.com/getsphericon/sphericon/internal/outbound"
)

// Active lists the holds of the scope's Workspace that are active at now, ordered by
// key. It is what the owner-facing and console reads of a Workspace's holds use.
func Active(ctx context.Context, s *ent.Scoped, now time.Time) ([]*ent.Hold, error) {
	return s.Hold().Query().
		Where(enthold.ExpiresAtGT(now)).
		Order(enthold.ByKey()).
		All(ctx)
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
	held, err := s.Hold().Query().Where(enthold.ExpiresAtGT(now())).Exist(ctx)
	if err != nil || !held {
		return "", err
	}
	return outbound.HoldOperator, nil
}
