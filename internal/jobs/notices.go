package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/usecase/noticedelivery"
)

// The Notices (ADR 0105): every mail sphericon sends on its own behalf, through the System sender,
// reacts to a domain event. The Notices' consumer wants the events Notice delivery covers; its job
// asks the use case for the Notices of the event and queues one send job per Notice, which the
// System sender delivers. A failed send is retried on its own, and a Notice queued again by a
// retried event job is the same send job (unique on the event and the address), so nobody is mailed
// twice for one event.

// NoticeEventArgs is the Notices' consumer's job: an event that calls for Notices.
type NoticeEventArgs struct {
	events.EventJob
}

func (NoticeEventArgs) Kind() string { return "event_notice" }

// callsForNotices wants an event that calls for Notices (noticedelivery.UseCase.Issue), reading the
// event alone, as a consumer's filter must: an account's event, a change of a Workspace its people
// are told of, and a Tracking host that fell back.
func callsForNotices(_ events.Envelope, ev events.DomainEvent) bool {
	switch e := ev.(type) {
	case *events.LinkRequested, *events.AddressClaimed, *events.UserSignedUp, *events.IdentityLinked,
		*events.UserDeleted, *events.PasskeyChange, *events.InvitationCreated, *events.SecondFactorRequired,
		*events.SecondFactorReminderDue, *events.WorkspaceSuspended, *events.AbuseWarned, *events.JoinDomainLost,
		*events.SendingDomainVerificationLost, *events.TrackingDomainReplaced, *events.TrackingDomainRemoved,
		*events.WebhookEndpointDisabled:
		return true
	case *events.TrackingDomainChanged:
		return e.Regressed
	}
	return false
}

// NoticeEventWorker turns an event into its Notices' send jobs. On an Instance with no System
// sender there is nothing to send them with, and it queues none.
type NoticeEventWorker struct {
	river.WorkerDefaults[NoticeEventArgs]

	notices *noticedelivery.UseCase
	sender  messaging.EmailSender
}

// NewNoticeEventWorker is the Notices' consumer's worker over Notice delivery and the System
// sender (nil on an Instance with no outbound mail).
func NewNoticeEventWorker(notices *noticedelivery.UseCase, sender messaging.EmailSender) *NoticeEventWorker {
	return &NoticeEventWorker{notices: notices, sender: sender}
}

func (w *NoticeEventWorker) Work(ctx context.Context, job *river.Job[NoticeEventArgs]) error {
	env := job.Args.Envelope
	if w.sender == nil {
		slog.InfoContext(ctx, "notice: no system sender, the event's notices are not sent", "event", env.Name, "event_id", env.ID)
		return nil
	}
	ev, err := events.Decode(env)
	if err != nil {
		return err
	}
	notices, err := w.notices.Issue(ctx, ev)
	if err != nil {
		return err
	}
	err = queueEach(ctx, notices, func(n noticedelivery.Notice) river.JobArgs {
		return SendNoticeArgs{EventID: env.ID, To: n.To, Subject: n.Subject, Text: n.Text}
	})
	if err != nil {
		return fmt.Errorf("queue the notices of %q: %w", env.Name, err)
	}
	return nil
}

// SendNoticeArgs is one Notice to send through the System sender. It is unique on the event and
// the address, so the Notices a retried event job queues again are the ones already queued.
type SendNoticeArgs struct {
	EventID string `json:"event_id" river:"unique"`
	To      string `json:"to" river:"unique"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
}

func (SendNoticeArgs) Kind() string { return "send_notice" }

func (SendNoticeArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// SendNoticeWorker sends a Notice through the System sender: sphericon's own provider, never a
// Workspace's Integration, so the mail bypasses the Workspace's suppression and eligibility
// (ADR 0015).
type SendNoticeWorker struct {
	river.WorkerDefaults[SendNoticeArgs]

	sender messaging.EmailSender
}

// NewSendNoticeWorker is the worker that sends the Notices through sender.
func NewSendNoticeWorker(sender messaging.EmailSender) *SendNoticeWorker {
	return &SendNoticeWorker{sender: sender}
}

func (w *SendNoticeWorker) Work(ctx context.Context, job *river.Job[SendNoticeArgs]) error {
	if w.sender == nil {
		return errors.New("send notice: no system email sender configured")
	}
	_, err := w.sender.Send(ctx, messaging.EmailMessage{To: job.Args.To, Subject: job.Args.Subject, Text: job.Args.Text})
	return err
}
