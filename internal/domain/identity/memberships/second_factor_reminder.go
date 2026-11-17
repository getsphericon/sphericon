package memberships

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
)

// ClaimSecondFactorReminder records on the Membership m, at the module's now, that its User is being reminded
// of the end of their Two-factor grace (ADR 0020), unless a reminder of the grace that began at
// graceStart is already recorded: one recorded before graceStart belongs to an earlier grace and
// does not count. false when another call claimed it first.
func (ms *Module) ClaimSecondFactorReminder(ctx context.Context, s *ent.Scoped, m *ent.Membership, graceStart time.Time) (bool, error) {
	claimed, err := s.Membership().Update().
		Where(membership.ID(m.ID), membership.Or(
			membership.SecondFactorRemindedAtIsNil(),
			membership.SecondFactorRemindedAtLT(graceStart),
		)).
		SetSecondFactorRemindedAt(ms.now()).
		Save(ctx)
	return claimed > 0, err
}
