package jobs

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domainverify"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// Inline is the synchronous adapter of the enqueue seam (the Rails `:inline`
// equivalent): instead of inserting into river, it runs each job's pure function
// immediately. It satisfies the same narrow enqueue interfaces the river Client
// does (WelcomeEnqueuer, SystemMailEnqueuer), so the test harness (and a future
// single-process mode) exercise the enqueue→execute path for real rather than
// asserting "a job was enqueued".
type Inline struct {
	ent          *ent.Client
	mod          *outbound.Module
	systemSender messaging.EmailSender
	domains      *domainverify.Module
	cipher       *secrets.Cipher
	catalog      *messaging.Catalog
	appURL       string
}

// NewInline builds the inline adapter. systemSender sends platform mail; mod is the
// Outbound send module every workspace email goes through. domains verifies Sending
// domains (a stub lookup in tests avoids real DNS). appURL builds account-email links.
func NewInline(entClient *ent.Client, mod *outbound.Module, systemSender messaging.EmailSender, domains *domainverify.Module, cipher *secrets.Cipher, catalog *messaging.Catalog, appURL string) *Inline {
	return &Inline{ent: entClient, mod: mod, systemSender: systemSender, domains: domains, cipher: cipher, catalog: catalog, appURL: appURL}
}

// EnqueueWelcome sends the welcome email now via the system sender.
func (i *Inline) EnqueueWelcome(ctx context.Context, email, name string) error {
	return SendWelcome(ctx, i.systemSender, email, name)
}

// EnqueuePasswordReset sends the password-reset email now, or only builds it when
// send is false.
func (i *Inline) EnqueuePasswordReset(ctx context.Context, email, token string, send bool) error {
	return SendAuthMail(ctx, i.systemSender, i.appURL, SendAuthMailArgs{Flow: flowPasswordReset, Email: email, Token: token, Discard: !send})
}

// EnqueueEmailVerification sends the email-verification email now, or only builds
// it when send is false.
func (i *Inline) EnqueueEmailVerification(ctx context.Context, email, token string, send bool) error {
	return SendAuthMail(ctx, i.systemSender, i.appURL, SendAuthMailArgs{Flow: flowEmailVerify, Email: email, Token: token, Discard: !send})
}

// EnqueueEmailChangeConfirm sends the confirm-new-email email now, or only builds it
// when send is false.
func (i *Inline) EnqueueEmailChangeConfirm(ctx context.Context, email, token string, send bool) error {
	return SendAuthMail(ctx, i.systemSender, i.appURL, SendAuthMailArgs{Flow: flowEmailChange, Email: email, Token: token, Discard: !send})
}

// EnqueueAddressTakenNotice sends the tokenless Taken-address notice now, or only
// builds it when send is false.
func (i *Inline) EnqueueAddressTakenNotice(ctx context.Context, email string, send bool) error {
	return SendAuthMail(ctx, i.systemSender, i.appURL, SendAuthMailArgs{Flow: flowAddressTaken, Email: email, Discard: !send})
}

// EnqueueIdentityLinkedNotice sends the Linked-identity notice now.
func (i *Inline) EnqueueIdentityLinkedNotice(ctx context.Context, email, provider, token string) error {
	return SendAuthMail(ctx, i.systemSender, i.appURL, SendAuthMailArgs{Flow: flowIdentityLinked, Email: email, Provider: provider, Token: token})
}

// EnqueueUserDeletionLink sends the deletion link mail now (nothing when send is false).
func (i *Inline) EnqueueUserDeletionLink(ctx context.Context, email, token string, send bool) error {
	return SendAuthMail(ctx, i.systemSender, i.appURL, SendAuthMailArgs{Flow: flowUserDeletion, Email: email, Token: token, Discard: !send})
}

// EnqueueUserDeletedNotice sends the User-deleted notice now.
func (i *Inline) EnqueueUserDeletedNotice(ctx context.Context, email string) error {
	return SendAuthMail(ctx, i.systemSender, i.appURL, SendAuthMailArgs{Flow: flowUserDeleted, Email: email})
}

// EnqueuePasskeyNotice sends the Passkey change notice now.
func (i *Inline) EnqueuePasskeyNotice(ctx context.Context, email string, change PasskeyChange, name string) error {
	return SendAuthMail(ctx, i.systemSender, i.appURL, SendAuthMailArgs{Flow: flowPasskeyChanged, Email: email, Change: change, Name: name})
}

// EnqueueSendingDomainVerify re-checks the domain's DKIM DNS now, notifying the
// workspace owner if this check flips the domain to unverified (ADR 0010 slice 3).
func (i *Inline) EnqueueSendingDomainVerify(ctx context.Context, sendingDomainID int64) error {
	_, err := i.domains.VerifyDKIM(ctx, sendingDomainID)
	return err
}

// VerifySendingDomain runs both checks of the domain now.
func (i *Inline) VerifySendingDomain(ctx context.Context, sendingDomainID int64) error {
	return i.domains.Verify(ctx, sendingDomainID)
}

// EnqueueIntegrationQuotaRefresh reads the Integration's provider send quota now.
func (i *Inline) EnqueueIntegrationQuotaRefresh(ctx context.Context, integrationID int64) error {
	return RefreshIntegrationQuotaByID(ctx, i.ent, i.cipher, i.catalog, integrationID)
}

// EnqueueMemberInvite sends the workspace invite email now via the system sender.
func (i *Inline) EnqueueMemberInvite(ctx context.Context, email, inviteURL, workspaceName, inviterName string) error {
	return SendMemberInvite(ctx, i.systemSender, SendMemberInviteArgs{
		Email:         email,
		InviteURL:     inviteURL,
		WorkspaceName: workspaceName,
		InviterName:   inviterName,
	})
}

// EnqueueSecondFactorRequired sends the "a Second factor is now required" email now.
func (i *Inline) EnqueueSecondFactorRequired(ctx context.Context, workspaceID int64) error {
	return NotifySecondFactorRequired(ctx, i.ent, i.systemSender, i.appURL, workspaceID)
}
