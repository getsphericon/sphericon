package jobs

import (
	"context"
	"errors"
	"fmt"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/mailjobs"
	"github.com/getsphericon/sphericon/internal/messaging"
)

// SendMemberInviteWorker sends the invite email via the system (platform) sender.
type SendMemberInviteWorker struct {
	river.WorkerDefaults[mailjobs.MemberInviteArgs]

	sender messaging.EmailSender
}

// NewMemberInviteWorker builds the worker that sends a Member invitation through the system sender.
func NewMemberInviteWorker(sender messaging.EmailSender) *SendMemberInviteWorker {
	return &SendMemberInviteWorker{sender: sender}
}

func (w *SendMemberInviteWorker) Work(ctx context.Context, job *river.Job[mailjobs.MemberInviteArgs]) error {
	args := job.Args
	if w.sender == nil {
		return errors.New("send member invite: no system email sender configured")
	}
	who := args.InviterName
	if who == "" {
		who = "Someone"
	}
	body := fmt.Sprintf(
		"%s invited you to join the \"%s\" workspace on sphericon.\n\nFollow this link to accept:\n\n%s\n\nIf you weren't expecting this, you can ignore this email.\n",
		who, args.WorkspaceName, args.InviteURL,
	)
	_, err := w.sender.Send(ctx, messaging.EmailMessage{
		To:      args.Email,
		Subject: fmt.Sprintf("You're invited to %s on sphericon", args.WorkspaceName),
		Text:    body,
	})
	return err
}
