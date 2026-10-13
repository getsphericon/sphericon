package jobs

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/messaging"
)

// Auth-mail flows: the self-service account emails sent through the system
// (platform) sender. Each maps to a public SPA page that posts the token back.
const (
	flowPasswordReset = "password_reset"
	flowEmailVerify   = "email_verify"
	flowEmailChange   = "email_change"
	// flowAddressTaken is the notice to the owner of a Taken address (ADR 0031): it
	// carries no token, only the sign-in and forgot-password pages.
	flowAddressTaken = "address_taken"
)

// SendAuthMailArgs is the river payload for a self-service account email. Flow
// selects the subject line and the SPA path the link points at.
type SendAuthMailArgs struct {
	Flow  string `json:"flow"`
	Email string `json:"email"`
	Token string `json:"token"`
	// Discard builds the mail and drops it. Forgot-password enqueues a Discard job
	// for an address that gets no mail (unknown, or over its budget), so every
	// request does the same work and the response time says nothing about the
	// address.
	Discard bool `json:"discard,omitempty"`
}

func (SendAuthMailArgs) Kind() string { return "send_auth_mail" }

// SendAuthMailWorker sends account emails via the system (platform) sender.
type SendAuthMailWorker struct {
	river.WorkerDefaults[SendAuthMailArgs]
	sender messaging.EmailSender
	appURL string
}

func (w *SendAuthMailWorker) Work(ctx context.Context, job *river.Job[SendAuthMailArgs]) error {
	return SendAuthMail(ctx, w.sender, w.appURL, job.Args)
}

// SendAuthMail renders and sends a self-service account email through the system
// sender (sphericon's own provider — NOT a workspace integration, so it bypasses the
// workspace suppression/eligibility machinery). Pure (no queue) so it runs
// identically under the river worker and the inline adapter.
func SendAuthMail(ctx context.Context, sender messaging.EmailSender, appURL string, args SendAuthMailArgs) error {
	if sender == nil {
		return fmt.Errorf("send auth mail: no system email sender configured")
	}
	msg, err := buildAuthMail(appURL, args)
	if err != nil || args.Discard {
		return err
	}
	_, err = sender.Send(ctx, msg)
	return err
}

// buildAuthMail renders an account email without sending it.
func buildAuthMail(appURL string, args SendAuthMailArgs) (messaging.EmailMessage, error) {
	if args.Flow == flowAddressTaken {
		return buildAddressTakenMail(appURL, args.Email), nil
	}
	subjectID, path, introID := authMailCopy(args.Flow)
	if path == "" {
		return messaging.EmailMessage{}, fmt.Errorf("send auth mail: unknown flow %q", args.Flow)
	}
	link := strings.TrimRight(appURL, "/") + path + "?token=" + url.QueryEscape(args.Token)
	body := fmt.Sprintf("%s\n\n%s\n\n%s\n", i18n.T(introID, nil), link, i18n.T("email.auth.footer", nil))
	return messaging.EmailMessage{To: args.Email, Subject: i18n.T(subjectID, nil), Text: body}, nil
}

// buildAddressTakenMail renders the notice a registration or an email change to a
// Taken address sends its owner. It carries no token, so it can never be a way in.
func buildAddressTakenMail(appURL, to string) messaging.EmailMessage {
	base := strings.TrimRight(appURL, "/")
	body := fmt.Sprintf("%s\n\n%s %s\n%s %s\n\n%s\n",
		i18n.T("email.address_taken.intro", nil),
		i18n.T("email.address_taken.sign_in", nil), base+"/login",
		i18n.T("email.address_taken.reset", nil), base+"/forgot-password",
		i18n.T("email.auth.footer", nil))
	return messaging.EmailMessage{To: to, Subject: i18n.T("email.address_taken.subject", nil), Text: body}
}

// authMailCopy returns the subject message id, SPA path, and intro message id
// for a flow (empty path signals an unknown flow). The copy itself is localized
// by internal/i18n at send time.
func authMailCopy(flow string) (subjectID, path, introID string) {
	switch flow {
	case flowPasswordReset:
		return "email.password_reset.subject", "/reset-password", "email.password_reset.intro"
	case flowEmailVerify:
		return "email.email_verify.subject", "/verify-email", "email.email_verify.intro"
	case flowEmailChange:
		return "email.email_change.subject", "/confirm-email-change", "email.email_change.intro"
	default:
		return "", "", ""
	}
}

// EnqueuePasswordReset schedules the password-reset email (river adapter). With
// send false the job is queued all the same but its worker drops the mail.
func (c *Client) EnqueuePasswordReset(ctx context.Context, email, token string, send bool) error {
	return c.enqueue(ctx, SendAuthMailArgs{Flow: flowPasswordReset, Email: email, Token: token, Discard: !send})
}

// EnqueueEmailVerification schedules the email-verification email. With send false
// the job is queued all the same but its worker drops the mail (the resend answer
// must not tell an Unverified User from anyone else by its response time).
func (c *Client) EnqueueEmailVerification(ctx context.Context, email, token string, send bool) error {
	return c.enqueue(ctx, SendAuthMailArgs{Flow: flowEmailVerify, Email: email, Token: token, Discard: !send})
}

// EnqueueEmailChangeConfirm schedules the confirm-new-email email (sent to the
// requested new address). With send false the job is queued all the same but its
// worker drops the mail.
func (c *Client) EnqueueEmailChangeConfirm(ctx context.Context, email, token string, send bool) error {
	return c.enqueue(ctx, SendAuthMailArgs{Flow: flowEmailChange, Email: email, Token: token, Discard: !send})
}

// EnqueueAddressTakenNotice schedules the tokenless notice to the owner of a Taken
// address. With send false the job is queued all the same but its worker drops the
// mail, so a request's work does not depend on whether the address was taken.
func (c *Client) EnqueueAddressTakenNotice(ctx context.Context, email string, send bool) error {
	return c.enqueue(ctx, SendAuthMailArgs{Flow: flowAddressTaken, Email: email, Discard: !send})
}

func (c *Client) enqueue(ctx context.Context, args SendAuthMailArgs) error {
	_, err := c.river.Insert(ctx, args, nil)
	return err
}
