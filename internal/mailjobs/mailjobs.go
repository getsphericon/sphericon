// Package mailjobs holds the river job arguments of the mails sphericon sends from its own
// system sender (the platform's provider, never a Workspace's Integration): the welcome, the
// self-service account mails, the member invite and the Second factor notice. It is a leaf
// package so the modules that queue them (the Sign-up, the /site handlers) do not import the
// workers (internal/jobs), which import those modules.
package mailjobs

import (
	"context"

	"github.com/riverqueue/river"
)

// Enqueuer queues a job: the river client in production, a test adapter that works each job
// through its real worker in tests.
type Enqueuer interface {
	Enqueue(ctx context.Context, args river.JobArgs) error
}

// WelcomeArgs is the platform welcome mail after a Sign-up.
type WelcomeArgs struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

func (WelcomeArgs) Kind() string { return "send_welcome" }

// Welcome is the welcome mail to email, greeting name.
func Welcome(email, name string) WelcomeArgs { return WelcomeArgs{Email: email, Name: name} }

// MemberInviteArgs is a Workspace member invitation. The invite URL is built by the handler
// (it carries the one-time token), so the job only renders and sends.
type MemberInviteArgs struct {
	Email         string `json:"email"`
	InviteURL     string `json:"invite_url"`
	WorkspaceName string `json:"workspace_name"`
	InviterName   string `json:"inviter_name"`
}

func (MemberInviteArgs) Kind() string { return "send_member_invite" }

// SecondFactorRequiredArgs is the "a Second factor is now required" mail, sent once when a
// Workspace's Two-factor requirement is turned on (ADR 0020).
type SecondFactorRequiredArgs struct {
	WorkspaceID int64 `json:"workspace_id"`
}

func (SecondFactorRequiredArgs) Kind() string { return "second_factor_required_notify" }

// SecondFactorRequired is the notice to the Users of the Workspace without a Second factor.
func SecondFactorRequired(workspaceID int64) SecondFactorRequiredArgs {
	return SecondFactorRequiredArgs{WorkspaceID: workspaceID}
}

// Flow is a self-service account mail: it selects the copy and the SPA page the link points at.
type Flow string

// The account mails.
const (
	FlowPasswordReset Flow = "password_reset"
	FlowEmailVerify   Flow = "email_verify"
	FlowEmailChange   Flow = "email_change"
	// FlowUserDeletion is the one-time link a User with no password confirms their User
	// deletion with (ADR 0050).
	FlowUserDeletion Flow = "user_deletion"
	// FlowAddressTaken is the notice to the owner of a Taken address (ADR 0031): it carries no
	// token, only the sign-in and forgot-password pages.
	FlowAddressTaken Flow = "address_taken"
	// FlowIdentityLinked is the notice that a Sign-in provider was linked to the account
	// (ADR 0048): Provider names it, Token is the "this was not me" link.
	FlowIdentityLinked Flow = "identity_linked"
	// FlowUserDeleted is the notice, to the address, that its User was deleted (ADR 0050): no
	// token and no link, so it can never be a way in.
	FlowUserDeleted Flow = "user_deleted"
	// FlowPasskeyChanged is the notice that a Passkey was added or removed (ADR 0052): Change
	// says which and Name carries the Passkey's name. No token and no link.
	FlowPasskeyChanged Flow = "passkey_changed"
)

// PasskeyChange is what a Passkey notice reports.
type PasskeyChange string

// PasskeyAdded and PasskeyRemoved are the changes a Passkey notice reports.
const (
	PasskeyAdded   PasskeyChange = "added"
	PasskeyRemoved PasskeyChange = "removed"
)

// AuthMailArgs is a self-service account mail.
type AuthMailArgs struct {
	Flow  Flow   `json:"flow"`
	Email string `json:"email"`
	Token string `json:"token"`
	// Provider is the display name of the Sign-in provider (identity_linked only).
	Provider string `json:"provider,omitempty"`
	// Change is PasskeyAdded or PasskeyRemoved, and Name the Passkey's name (passkey_changed only).
	Change PasskeyChange `json:"change,omitempty"`
	Name   string        `json:"name,omitempty"`
	// Discard builds the mail and drops it. A request that must not reveal whether an address
	// gets mail (forgot-password for an unknown address, or one over its budget) queues a
	// Discard job, so every request does the same work and the response time says nothing.
	Discard bool `json:"discard,omitempty"`
}

func (AuthMailArgs) Kind() string { return "send_auth_mail" }

// PasswordReset is the reset link to email; with send false the job builds the mail and drops it.
func PasswordReset(email, token string, send bool) AuthMailArgs {
	return AuthMailArgs{Flow: FlowPasswordReset, Email: email, Token: token, Discard: !send}
}

// EmailVerification is the verification link to email; with send false it is dropped.
func EmailVerification(email, token string, send bool) AuthMailArgs {
	return AuthMailArgs{Flow: FlowEmailVerify, Email: email, Token: token, Discard: !send}
}

// EmailChangeConfirm is the confirm-new-address link, sent to the requested address; with send
// false it is dropped.
func EmailChangeConfirm(email, token string, send bool) AuthMailArgs {
	return AuthMailArgs{Flow: FlowEmailChange, Email: email, Token: token, Discard: !send}
}

// UserDeletionLink is the deletion link of a User with no password; with send false it is dropped.
func UserDeletionLink(email, token string, send bool) AuthMailArgs {
	return AuthMailArgs{Flow: FlowUserDeletion, Email: email, Token: token, Discard: !send}
}

// AddressTakenNotice is the tokenless notice to the owner of a Taken address; with send false
// it is dropped, so a request's work does not depend on whether the address was taken.
func AddressTakenNotice(email string, send bool) AuthMailArgs {
	return AuthMailArgs{Flow: FlowAddressTaken, Email: email, Discard: !send}
}

// IdentityLinkedNotice is the notice that provider was linked to the account silently, with
// the token of its "this was not me" link.
func IdentityLinkedNotice(email, provider, token string) AuthMailArgs {
	return AuthMailArgs{Flow: FlowIdentityLinked, Email: email, Provider: provider, Token: token}
}

// UserDeletedNotice is the notice that the User with this address was deleted.
func UserDeletedNotice(email string) AuthMailArgs {
	return AuthMailArgs{Flow: FlowUserDeleted, Email: email}
}

// PasskeyNotice is the notice that the Passkey name was added or removed.
func PasskeyNotice(email string, change PasskeyChange, name string) AuthMailArgs {
	return AuthMailArgs{Flow: FlowPasskeyChanged, Email: email, Change: change, Name: name}
}
