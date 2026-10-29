package jobs

import (
	"context"
	"errors"
	"fmt"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/messaging"
)

// SendMemberInviteArgs is the river payload for a workspace member invitation
// email. The invite URL is prebuilt by the handler (it embeds the one-time
// token), so the job only renders and sends.
type SendMemberInviteArgs struct {
	Email         string `json:"email"`
	InviteURL     string `json:"invite_url"`
	WorkspaceName string `json:"workspace_name"`
	InviterName   string `json:"inviter_name"`
}

func (SendMemberInviteArgs) Kind() string { return "send_member_invite" }

// SendMemberInviteWorker sends the invite email via the system (platform) sender.
type SendMemberInviteWorker struct {
	river.WorkerDefaults[SendMemberInviteArgs]

	sender messaging.EmailSender
}

// NewMemberInviteWorker builds the worker that sends a Member invitation through the system sender.
func NewMemberInviteWorker(sender messaging.EmailSender) *SendMemberInviteWorker {
	return &SendMemberInviteWorker{sender: sender}
}

func (w *SendMemberInviteWorker) Work(ctx context.Context, job *river.Job[SendMemberInviteArgs]) error {
	return SendMemberInvite(ctx, w.sender, job.Args)
}

// SendMemberInvite renders and sends a workspace invite through the system sender.
// Pure (no queue) so it runs identically under the river worker and the inline
// adapter.
func SendMemberInvite(ctx context.Context, sender messaging.EmailSender, args SendMemberInviteArgs) error {
	if sender == nil {
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
	_, err := sender.Send(ctx, messaging.EmailMessage{
		To:      args.Email,
		Subject: fmt.Sprintf("You're invited to %s on sphericon", args.WorkspaceName),
		Text:    body,
	})
	return err
}

// EnqueueMemberInvite schedules the invite email (river adapter).
func (c *Client) EnqueueMemberInvite(ctx context.Context, email, inviteURL, workspaceName, inviterName string) error {
	_, err := c.river.Insert(ctx, SendMemberInviteArgs{
		Email:         email,
		InviteURL:     inviteURL,
		WorkspaceName: workspaceName,
		InviterName:   inviterName,
	}, nil)
	return err
}
