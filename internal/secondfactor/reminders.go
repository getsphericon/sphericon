package secondfactor

import (
	"context"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/accounts"
)

// ReminderLead is how long before a User's grace ends they are reminded to set up a
// Second factor (ADR 0020).
const ReminderLead = 24 * time.Hour

// SendReminder delivers the reminder of one Membership whose grace ends at deadline.
type SendReminder func(ctx context.Context, m *ent.Membership, deadline time.Time) error

// RemindDeadlines reminds, once per grace, every User without a Second factor whose grace in
// a Workspace with a Two-factor requirement ends within ReminderLead of now. The Membership
// records the reminder: it is claimed before send runs and released if send fails, so a later
// call retries it and a concurrent or repeated call never sends it twice. A reminder from an
// earlier grace (the requirement switched off and on again) does not count. It lists the
// Memberships of every Workspace, so it works on the raw client; each claim is scoped to the
// Workspace of the Membership it loaded.
func RemindDeadlines(ctx context.Context, client *ent.Client, now time.Time, send SendReminder) error {
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
		deadline, ok := accounts.SecondFactorDeadline(m)
		if !ok || now.Before(deadline.Add(-ReminderLead)) || !now.Before(deadline) {
			continue
		}
		if err := remindOnce(ctx, client.Scoped(m.WorkspaceID), m, deadline, now, send); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// remindOnce claims the Membership's reminder for the grace that ends at deadline, sends it,
// and releases the claim if the send fails.
func remindOnce(ctx context.Context, s *ent.Scoped, m *ent.Membership, deadline, now time.Time, send SendReminder) error {
	graceStart := deadline.Add(-accounts.GracePeriod)
	claimed, err := s.Membership().Update().
		Where(membership.ID(m.ID), membership.Or(
			membership.SecondFactorRemindedAtIsNil(),
			membership.SecondFactorRemindedAtLT(graceStart),
		)).
		SetSecondFactorRemindedAt(now).
		Save(ctx)
	if err != nil || claimed == 0 {
		return err
	}
	serr := send(ctx, m, deadline)
	if serr == nil {
		return nil
	}
	release := s.Membership().UpdateOneID(m.ID)
	if m.SecondFactorRemindedAt == nil {
		release = release.ClearSecondFactorRemindedAt()
	} else {
		release = release.SetSecondFactorRemindedAt(*m.SecondFactorRemindedAt)
	}
	return errors.Join(serr, release.Exec(ctx))
}
