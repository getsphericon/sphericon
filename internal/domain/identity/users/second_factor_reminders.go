package users

import (
	"context"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// ReminderLead is how long before a User's grace ends they are reminded to set up a
// Second factor (ADR 0020).
const ReminderLead = 24 * time.Hour

// RemindSecondFactorDeadlines reminds, once per grace, every User without a Second factor whose
// grace in a Workspace with a Two-factor requirement ends within ReminderLead of now. The
// Membership records the reminder (memberships.ClaimSecondFactorReminder) and the claim publishes
// events.SecondFactorReminderDue in its transaction, so a concurrent or repeated call never
// reminds twice and the reminder's Notice exists iff the claim commits (ADR 0105). A reminder from
// an earlier grace (the requirement switched off and on again) does not count. It lists the
// Memberships of every Workspace, so it works on the raw client; each claim is scoped to the
// Workspace of the Membership it loaded.
func RemindSecondFactorDeadlines(ctx context.Context, client *ent.Client, bus *events.Bus, now time.Time) error {
	members, err := client.Membership.Query().
		Where(membership.HasWorkspaceWith(workspace.SecondFactorRequiredAtNotNil())).
		WithUser().
		WithWorkspace().
		All(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, m := range members {
		deadline, ok := SecondFactorDeadline(m)
		if !ok || now.Before(deadline.Add(-ReminderLead)) || !now.Before(deadline) {
			continue
		}
		if err := remindOnce(ctx, bus, client.Scoped(m.WorkspaceID), m, deadline, now); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// remindOnce claims the Membership's reminder for the grace that ends at deadline and publishes
// it, in one transaction.
func remindOnce(ctx context.Context, bus *events.Bus, s *ent.Scoped, m *ent.Membership, deadline, now time.Time) error {
	return bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		claimed, err := memberships.ClaimSecondFactorReminder(ctx, ts, m, deadline.Add(-GracePeriod), now)
		if err != nil || !claimed {
			return err
		}
		return pub.Publish(ctx, &events.SecondFactorReminderDue{WorkspaceID: m.WorkspaceID, MembershipID: m.ID, Deadline: deadline})
	})
}
