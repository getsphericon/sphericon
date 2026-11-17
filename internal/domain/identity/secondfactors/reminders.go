package secondfactors

import (
	"context"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// ReminderLead is how long before a User's grace ends they are reminded to set up a
// Second factor (ADR 0020).
const ReminderLead = 24 * time.Hour

// RemindDeadlines reminds, once per grace, every User without a Second factor whose grace in a
// Workspace with a Two-factor requirement ends within ReminderLead of now (the module's clock). The
// Membership records the reminder (memberships.Module.ClaimSecondFactorReminder) and the claim publishes
// events.SecondFactorReminderDue in its transaction, so a concurrent or repeated call never
// reminds twice and the reminder's Notice exists iff the claim commits (ADR 0105). A reminder from
// an earlier grace (the requirement switched off and on again) does not count. It lists the
// Memberships of every Workspace, so it works on the raw client; each claim is scoped to the
// Workspace of the Membership it loaded.
func (m *Module) RemindDeadlines(ctx context.Context) error {
	now := m.now()
	members, err := m.ent.Membership.Query().
		Where(membership.HasWorkspaceWith(workspace.SecondFactorRequiredAtNotNil())).
		WithWorkspace().
		All(ctx)
	if err != nil {
		return err
	}
	ends, err := deadlines(ctx, m.ent, members)
	if err != nil {
		return err
	}
	var errs []error
	for _, ms := range members {
		deadline, ok := ends[ms.ID]
		if !ok || now.Before(deadline.Add(-ReminderLead)) || !now.Before(deadline) {
			continue
		}
		if err := m.remindOnce(ctx, m.ent.Scoped(ms.WorkspaceID), ms, deadline); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// remindOnce claims the Membership's reminder for the grace that ends at deadline and publishes
// it, in one transaction.
func (m *Module) remindOnce(ctx context.Context, s *ent.Scoped, ms *ent.Membership, deadline time.Time) error {
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		claimed, err := m.members.ClaimSecondFactorReminder(ctx, ts, ms, deadline.Add(-GracePeriod))
		if err != nil || !claimed {
			return err
		}
		return pub.Publish(ctx, &events.SecondFactorReminderDue{WorkspaceID: ms.WorkspaceID, MembershipID: ms.ID, Deadline: deadline})
	})
}
