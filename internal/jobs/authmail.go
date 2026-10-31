package jobs

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/mailjobs"
	"github.com/getsphericon/sphericon/internal/messaging"
)

// SendAuthMailWorker sends account emails via the system (platform) sender.
type SendAuthMailWorker struct {
	river.WorkerDefaults[mailjobs.AuthMailArgs]

	sender messaging.EmailSender
	appURL string
}

// NewAuthMailWorker builds the worker that sends account emails, their links built on appURL.
func NewAuthMailWorker(sender messaging.EmailSender, appURL string) *SendAuthMailWorker {
	return &SendAuthMailWorker{sender: sender, appURL: appURL}
}

func (w *SendAuthMailWorker) Work(ctx context.Context, job *river.Job[mailjobs.AuthMailArgs]) error {
	// The system sender is sphericon's own provider, not a Workspace's Integration, so the
	// mail bypasses the Workspace's suppression and eligibility.
	if w.sender == nil {
		return errors.New("send auth mail: no system email sender configured")
	}
	msg, err := buildAuthMail(w.appURL, job.Args)
	if err != nil || job.Args.Discard {
		return err
	}
	_, err = w.sender.Send(ctx, msg)
	return err
}

// linkMail is the copy of an account mail that carries a one-time link: its subject, the SPA
// page the link opens (which posts the token back) and the line above the link. The copy is
// localized by internal/i18n at send time.
type linkMail struct {
	subject, intro i18n.ID
	path           string
}

// linkMails is the copy of every account mail with a link.
var linkMails = map[mailjobs.Flow]linkMail{
	mailjobs.FlowPasswordReset: {subject: i18n.EmailPasswordResetSubject, path: "/reset-password", intro: i18n.EmailPasswordResetIntro},
	mailjobs.FlowEmailVerify:   {subject: i18n.EmailEmailVerifySubject, path: "/verify-email", intro: i18n.EmailEmailVerifyIntro},
	mailjobs.FlowEmailChange:   {subject: i18n.EmailEmailChangeSubject, path: "/confirm-email-change", intro: i18n.EmailEmailChangeIntro},
	mailjobs.FlowUserDeletion:  {subject: i18n.EmailUserDeletionSubject, path: "/delete-account", intro: i18n.EmailUserDeletionIntro},
}

// notices renders the account mails that are not a plain link, each in its own shape.
var notices = map[mailjobs.Flow]func(appURL string, args mailjobs.AuthMailArgs) (messaging.EmailMessage, error){
	mailjobs.FlowAddressTaken:   buildAddressTakenMail,
	mailjobs.FlowIdentityLinked: buildIdentityLinkedMail,
	mailjobs.FlowPasskeyChanged: buildPasskeyChangedMail,
	mailjobs.FlowUserDeleted:    buildUserDeletedMail,
}

// buildAuthMail renders an account email without sending it.
func buildAuthMail(appURL string, args mailjobs.AuthMailArgs) (messaging.EmailMessage, error) {
	if build, ok := notices[args.Flow]; ok {
		return build(appURL, args)
	}
	c, ok := linkMails[args.Flow]
	if !ok {
		return messaging.EmailMessage{}, fmt.Errorf("send auth mail: unknown flow %q", args.Flow)
	}
	link := strings.TrimRight(appURL, "/") + c.path + "?token=" + url.QueryEscape(args.Token)
	body := fmt.Sprintf("%s\n\n%s\n\n%s\n", i18n.T(c.intro, nil), link, i18n.T(i18n.EmailAuthFooter, nil))
	return messaging.EmailMessage{To: args.Email, Subject: i18n.T(c.subject, nil), Text: body}, nil
}

// buildAddressTakenMail renders the notice a Sign-up or an email change to a
// Taken address sends its owner. It carries no token, so it can never be a way in.
func buildAddressTakenMail(appURL string, args mailjobs.AuthMailArgs) (messaging.EmailMessage, error) {
	base := strings.TrimRight(appURL, "/")
	body := fmt.Sprintf("%s\n\n%s %s\n%s %s\n\n%s\n",
		i18n.T(i18n.EmailAddressTakenIntro, nil),
		i18n.T(i18n.EmailAddressTakenSignIn, nil), base+"/login",
		i18n.T(i18n.EmailAddressTakenReset, nil), base+"/forgot-password",
		i18n.T(i18n.EmailAuthFooter, nil))
	return messaging.EmailMessage{To: args.Email, Subject: i18n.T(i18n.EmailAddressTakenSubject, nil), Text: body}, nil
}

// buildUserDeletedMail renders the one mail a User deletion sends the address: the owner of
// the mailbox learns of a deletion they did not make.
func buildUserDeletedMail(_ string, args mailjobs.AuthMailArgs) (messaging.EmailMessage, error) {
	body := fmt.Sprintf("%s\n\n%s\n", i18n.T(i18n.EmailUserDeletedIntro, nil), i18n.T(i18n.EmailUserDeletedNotMe, nil))
	return messaging.EmailMessage{To: args.Email, Subject: i18n.T(i18n.EmailUserDeletedSubject, nil), Text: body}, nil
}

// passkeyCopy is the subject and intro of a Passkey notice by the change it reports.
var passkeyCopy = map[mailjobs.PasskeyChange][2]i18n.ID{
	mailjobs.PasskeyAdded:   {i18n.EmailPasskeyAddedSubject, i18n.EmailPasskeyAddedIntro},
	mailjobs.PasskeyRemoved: {i18n.EmailPasskeyRemovedSubject, i18n.EmailPasskeyRemovedIntro},
}

// buildPasskeyChangedMail renders the notice a Passkey change sends the owner of the account.
func buildPasskeyChangedMail(_ string, args mailjobs.AuthMailArgs) (messaging.EmailMessage, error) {
	c, ok := passkeyCopy[args.Change]
	if !ok {
		return messaging.EmailMessage{}, fmt.Errorf("send auth mail: unknown passkey change %q", args.Change)
	}
	body := fmt.Sprintf("%s\n\n%s\n", i18n.T(c[1], map[string]any{"Name": args.Name}), i18n.T(i18n.EmailPasskeyChangedNotMe, nil))
	return messaging.EmailMessage{To: args.Email, Subject: i18n.T(c[0], nil), Text: body}, nil
}

// buildIdentityLinkedMail renders the notice a silent link of a Sign-in provider sends the
// owner of the account, with the link that undoes it.
func buildIdentityLinkedMail(appURL string, args mailjobs.AuthMailArgs) (messaging.EmailMessage, error) {
	link := strings.TrimRight(appURL, "/") + "/signin-not-me?token=" + url.QueryEscape(args.Token)
	body := fmt.Sprintf("%s\n\n%s\n%s\n\n%s\n",
		i18n.T(i18n.EmailIdentityLinkedIntro, map[string]any{"Provider": args.Provider}),
		i18n.T(i18n.EmailIdentityLinkedNotMe, nil), link,
		i18n.T(i18n.EmailAuthFooter, nil))
	return messaging.EmailMessage{To: args.Email, Subject: i18n.T(i18n.EmailIdentityLinkedSubject, nil), Text: body}, nil
}

// Enqueue queues one job of args. It is the one enqueue method the modules that queue
// platform mail use (mailjobs.Enqueuer).
func (c *Client) Enqueue(ctx context.Context, args river.JobArgs) error {
	_, err := c.river.Insert(ctx, args, nil)
	return err
}
