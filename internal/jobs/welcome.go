package jobs

import (
	"context"
	"errors"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/mailjobs"
	"github.com/getsphericon/sphericon/internal/messaging"
)

// SendWelcomeWorker sends the welcome email via the system (platform) sender.
type SendWelcomeWorker struct {
	river.WorkerDefaults[mailjobs.WelcomeArgs]

	sender messaging.EmailSender
}

// NewWelcomeWorker builds the worker that sends the welcome email through the system sender.
func NewWelcomeWorker(sender messaging.EmailSender) *SendWelcomeWorker {
	return &SendWelcomeWorker{sender: sender}
}

func (w *SendWelcomeWorker) Work(ctx context.Context, job *river.Job[mailjobs.WelcomeArgs]) error {
	if w.sender == nil {
		return errors.New("send welcome: no system email sender configured")
	}
	greeting := job.Args.Name
	if greeting == "" {
		greeting = i18n.T(i18n.EmailWelcomeGreetingFallback, nil)
	}
	body := i18n.T(i18n.EmailWelcomeBody, map[string]any{"Greeting": greeting})
	_, err := w.sender.Send(ctx, messaging.EmailMessage{
		To:      job.Args.Email,
		Subject: i18n.T(i18n.EmailWelcomeSubject, nil),
		Text:    body,
	})
	return err
}
