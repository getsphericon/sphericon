package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/secondfactor"
)

// SecondFactorReminderLead is how long before a User's grace ends they are reminded to
// set up a Second factor (ADR 0020).
const SecondFactorReminderLead = 24 * time.Hour

// secondFactorReminderInterval is how often the reminder sweep runs: a User is
// reminded within this long of their reminder becoming due.
const secondFactorReminderInterval = 15 * time.Minute

// secondFactorSetupPath is the SPA page where a User enrolls a Second factor.
const secondFactorSetupPath = "/account/security"

// NotifySecondFactorRequiredArgs is the river payload of the "a Second factor is now
// required" email, sent once when a Workspace's Two-factor requirement is turned on
// (ADR 0020).
type NotifySecondFactorRequiredArgs struct {
	WorkspaceID int64 `json:"workspace_id"`
}

func (NotifySecondFactorRequiredArgs) Kind() string { return "second_factor_required_notify" }

// NotifySecondFactorRequiredWorker sends that email via the system (platform) sender.
type NotifySecondFactorRequiredWorker struct {
	river.WorkerDefaults[NotifySecondFactorRequiredArgs]
	ent    *ent.Client
	sender messaging.EmailSender
	appURL string
}

func (w *NotifySecondFactorRequiredWorker) Work(ctx context.Context, job *river.Job[NotifySecondFactorRequiredArgs]) error {
	return NotifySecondFactorRequired(ctx, w.ent, w.sender, w.appURL, job.Args.WorkspaceID)
}

// NotifySecondFactorRequired emails every User of the Workspace who has no Second
// factor that the Workspace now requires one, with their deadline. The Memberships
// are read when the job runs, so a requirement switched off again in between mails no
// one. Platform mail goes through the system sender, outside Outbound send (ADR 0015).
// Pure (no queue) so it runs identically under the river worker and the inline adapter.
func NotifySecondFactorRequired(ctx context.Context, client *ent.Client, sender messaging.EmailSender, appURL string, workspaceID int64) error {
	if sender == nil {
		return fmt.Errorf("notify second factor required: no system email sender configured")
	}
	members, err := client.Scoped(workspaceID).Membership().Query().WithUser().WithWorkspace().All(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, m := range members {
		deadline, ok := secondfactor.Deadline(m)
		if !ok {
			continue
		}
		if err := sendSecondFactorMail(ctx, sender, appURL, "email.second_factor_required", m, deadline); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// sendSecondFactorMail renders the email of the given message id prefix (subject and
// body) for the Membership's User and sends it. The deadline is shown in UTC: the
// locale is the instance's, the User's time zone is unknown.
func sendSecondFactorMail(ctx context.Context, sender messaging.EmailSender, appURL, prefix string, m *ent.Membership, deadline time.Time) error {
	u := m.Edges.User
	data := map[string]any{
		"Workspace": m.Edges.Workspace.Name,
		"Deadline":  deadline.UTC().Format("2006-01-02 15:04 UTC"),
		"Link":      strings.TrimRight(appURL, "/") + secondFactorSetupPath,
	}
	if _, err := sender.Send(ctx, messaging.EmailMessage{
		To:      u.Email,
		Subject: i18n.T(prefix+".subject", data),
		Text:    i18n.T(prefix+".body", data),
	}); err != nil {
		return fmt.Errorf("mail %s: %w", u.Email, err)
	}
	return nil
}

// EnqueueSecondFactorRequired schedules the "a Second factor is now required" email
// for the Workspace's Users (river adapter).
func (c *Client) EnqueueSecondFactorRequired(ctx context.Context, workspaceID int64) error {
	_, err := c.river.Insert(ctx, NotifySecondFactorRequiredArgs{WorkspaceID: workspaceID}, nil)
	return err
}

// RemindSecondFactorArgs is the periodic tick that reminds Users whose grace under a
// Two-factor requirement ends within SecondFactorReminderLead (ADR 0020).
type RemindSecondFactorArgs struct{}

func (RemindSecondFactorArgs) Kind() string { return "second_factor_remind" }

// RemindSecondFactorWorker runs the reminder sweep on the wall clock.
type RemindSecondFactorWorker struct {
	river.WorkerDefaults[RemindSecondFactorArgs]
	ent    *ent.Client
	sender messaging.EmailSender
	appURL string
}

func (w *RemindSecondFactorWorker) Work(ctx context.Context, _ *river.Job[RemindSecondFactorArgs]) error {
	return RemindSecondFactorDeadlines(ctx, w.ent, w.sender, w.appURL, time.Now())
}

// RemindSecondFactorDeadlines emails, once per grace, every User without a Second
// factor whose grace in a Workspace with a Two-factor requirement ends within
// SecondFactorReminderLead of now. The Membership records the reminder: it is claimed
// before the send and released if the send fails, so a later tick retries it and a
// concurrent or repeated tick never sends it twice. A reminder from an earlier grace
// (the requirement switched off and on again) does not count.
func RemindSecondFactorDeadlines(ctx context.Context, client *ent.Client, sender messaging.EmailSender, appURL string, now time.Time) error {
	if sender == nil {
		return fmt.Errorf("remind second factor: no system email sender configured")
	}
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
		deadline, ok := secondfactor.Deadline(m)
		if !ok || now.Before(deadline.Add(-SecondFactorReminderLead)) || !now.Before(deadline) {
			continue
		}
		if err := remindOnce(ctx, client.Scoped(m.WorkspaceID), sender, appURL, m, deadline, now); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// remindOnce claims the Membership's reminder for the grace that ends at deadline,
// sends it, and releases the claim if the send fails.
func remindOnce(ctx context.Context, s *ent.Scoped, sender messaging.EmailSender, appURL string, m *ent.Membership, deadline, now time.Time) error {
	graceStart := deadline.Add(-secondfactor.GracePeriod)
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
	serr := sendSecondFactorMail(ctx, sender, appURL, "email.second_factor_reminder", m, deadline)
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
