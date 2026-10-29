package jobs

import (
	"context"
	"errors"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/messaging"
)

// SendWelcomeArgs is the river job payload for the platform welcome email.
type SendWelcomeArgs struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

func (SendWelcomeArgs) Kind() string { return "send_welcome" }

// SendWelcomeWorker sends the welcome email via the system (platform) sender.
type SendWelcomeWorker struct {
	river.WorkerDefaults[SendWelcomeArgs]

	sender messaging.EmailSender
}

// NewWelcomeWorker builds the worker that sends the welcome email through the system sender.
func NewWelcomeWorker(sender messaging.EmailSender) *SendWelcomeWorker {
	return &SendWelcomeWorker{sender: sender}
}

func (w *SendWelcomeWorker) Work(ctx context.Context, job *river.Job[SendWelcomeArgs]) error {
	return SendWelcome(ctx, w.sender, job.Args.Email, job.Args.Name)
}

// SendWelcome sends the platform welcome email through the system sender (sphericon's
// own provider — NOT a workspace integration). Pure (no queue) so it runs
// identically under the river worker and the inline adapter.
func SendWelcome(ctx context.Context, sender messaging.EmailSender, email, name string) error {
	if sender == nil {
		return errors.New("send welcome: no system email sender configured")
	}
	greeting := name
	if greeting == "" {
		greeting = i18n.T("email.welcome.greeting_fallback", nil)
	}
	body := i18n.T("email.welcome.body", map[string]any{"Greeting": greeting})
	_, err := sender.Send(ctx, messaging.EmailMessage{
		To:      email,
		Subject: i18n.T("email.welcome.subject", nil),
		Text:    body,
	})
	return err
}

// EnqueueWelcome schedules the welcome email (river adapter).
func (c *Client) EnqueueWelcome(ctx context.Context, email, name string) error {
	_, err := c.river.Insert(ctx, SendWelcomeArgs{Email: email, Name: name}, nil)
	return err
}
