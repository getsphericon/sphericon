package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/mailjobs"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/ownernotice"
	"github.com/getsphericon/sphericon/internal/reportday"
	"github.com/getsphericon/sphericon/internal/secondfactor"
)

// secondFactorReminderInterval is how often the reminder sweep runs: a User is
// reminded within this long of their reminder becoming due.
const secondFactorReminderInterval = 15 * time.Minute

// secondFactorSetupPath is the SPA page where a User enrolls a Second factor.
const secondFactorSetupPath = "/account/security"

// NotifySecondFactorRequiredWorker sends that email via the system (platform) sender.
type NotifySecondFactorRequiredWorker struct {
	river.WorkerDefaults[mailjobs.SecondFactorRequiredArgs]

	ent    *ent.Client
	sender messaging.EmailSender
	appURL string
}

// NewNotifySecondFactorRequiredWorker builds the worker that mails a Workspace's Users without a
// Second factor that it now requires one; links are built on appURL.
func NewNotifySecondFactorRequiredWorker(client *ent.Client, sender messaging.EmailSender, appURL string) *NotifySecondFactorRequiredWorker {
	return &NotifySecondFactorRequiredWorker{ent: client, sender: sender, appURL: appURL}
}

func (w *NotifySecondFactorRequiredWorker) Work(ctx context.Context, job *river.Job[mailjobs.SecondFactorRequiredArgs]) error {
	// Every User of the Workspace who has no Second factor is mailed their deadline. The
	// Memberships are read when the job runs, so a requirement switched off again in between
	// mails no one. Platform mail goes through the system sender, outside Outbound send (ADR 0015).
	if w.sender == nil {
		return errors.New("notify second factor required: no system email sender configured")
	}
	members, err := w.ent.Scoped(job.Args.WorkspaceID).Membership().Query().WithUser().WithWorkspace().All(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, m := range members {
		deadline, ok := accounts.SecondFactorDeadline(m)
		if !ok {
			continue
		}
		if err := sendSecondFactorMail(ctx, w.sender, w.appURL, ownernotice.Notice{Subject: i18n.EmailSecondFactorRequiredSubject, Body: i18n.EmailSecondFactorRequiredBody}, m, deadline); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// sendSecondFactorMail renders the mail (its subject and body messages)
// for the Membership's User and sends it. The deadline is shown in the User time zone
// (ADR 0068); the locale is the instance's.
func sendSecondFactorMail(ctx context.Context, sender messaging.EmailSender, appURL string, mail ownernotice.Notice, m *ent.Membership, deadline time.Time) error {
	u := m.Edges.User
	data := map[string]any{
		"Workspace": m.Edges.Workspace.Name,
		"Deadline":  mailTime(deadline, u.Timezone),
		"Link":      strings.TrimRight(appURL, "/") + secondFactorSetupPath,
	}
	if _, err := sender.Send(ctx, messaging.EmailMessage{
		To:      u.Email,
		Subject: i18n.T(mail.Subject, data),
		Text:    i18n.T(mail.Body, data),
	}); err != nil {
		return fmt.Errorf("mail %s: %w", u.Email, err)
	}
	return nil
}

// RemindSecondFactorArgs is the periodic tick that reminds Users whose grace under a
// Two-factor requirement ends within secondfactor.ReminderLead (ADR 0020).
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

// RemindSecondFactorDeadlines mails the reminder of every Membership the second-factor
// module claims for this tick (secondfactor.RemindDeadlines).
func RemindSecondFactorDeadlines(ctx context.Context, client *ent.Client, sender messaging.EmailSender, appURL string, now time.Time) error {
	if sender == nil {
		return errors.New("remind second factor: no system email sender configured")
	}
	return secondfactor.RemindDeadlines(ctx, client, now, func(ctx context.Context, m *ent.Membership, deadline time.Time) error {
		return sendSecondFactorMail(ctx, sender, appURL, ownernotice.Notice{Subject: i18n.EmailSecondFactorReminderSubject, Body: i18n.EmailSecondFactorReminderBody}, m, deadline)
	})
}

// mailTime is t as a User's mail prints it: the wall clock in the User time zone (ADR 0068) and
// the zone's name, which, unlike an abbreviation, every zone has. A User with none, or with a
// name the zone database no longer knows, reads UTC.
func mailTime(t time.Time, zone *string) string {
	loc, err := reportday.Location(zone)
	if err != nil {
		loc = time.UTC
	}
	return t.In(loc).Format("2006-01-02 15:04") + " " + loc.String()
}
