package users

import (
	"context"
	"log/slog"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
)

// Requests are the account's requests for a mailed link (ADR 0030, ADR 0031, ADR 0050): a password
// reset, the verification link sent again, an email change and the deletion link of a User with no
// password. A request changes no state of its own: it takes a slot of the address's mail budget
// where the request has one, and publishes its event in a transaction of its own
// (events.LinkRequested, events.AddressClaimed), which the Notices mail with a link minted then
// (ADR 0105).
//
// A request that must not tell whether an address holds a User does the same work for every
// address (a slot of the budget, a lookup, a published event) and publishes its event with Deliver
// false where no mail is due. Publishing is best-effort for the same reason: a failure is logged,
// never answered (it would differ from the answer the other addresses get), and a lost request is
// asked for again.
type Requests struct {
	users    *Module
	attempts *throttle.Attempts
}

// NewRequests builds the requests over the Users and the mail budgets of the addresses.
func NewRequests(a *Module, attempts *throttle.Attempts) *Requests {
	return &Requests{users: a, attempts: attempts}
}

// PasswordReset asks for a reset link to the address typed. Every address takes a slot of its
// budget and is looked up; only one that holds a User and was granted the slot is mailed, and its
// link, bound to the password as it is then, works once. A blank address asks for nothing.
func (r *Requests) PasswordReset(ctx context.Context, typed string) error {
	email := strings.TrimSpace(typed)
	if email == "" {
		return nil
	}
	// Unknown addresses spend their budget exactly like known ones.
	granted, err := r.attempts.Take(ctx, throttle.KindPasswordReset, email)
	if err != nil {
		return err
	}
	u, found, err := r.holder(ctx, email)
	if err != nil {
		return err
	}
	request := &events.LinkRequested{Action: events.NameUserPasswordResetRequested, Email: email}
	if found {
		request.UserID, request.Email, request.Deliver = u.ID, u.Email, granted
	}
	r.publish(ctx, request)
	return nil
}

// Verification asks for the verification link of the address typed again, the same link as at
// Sign-up: it works until the password is replaced or the address verified, and it trusts the
// Sign-up browser whose cookie value is browser ("" for none; then the link asks for a password).
// Every address is looked up and hashes the cookie the same way; only an Unverified User is
// mailed. A blank address asks for nothing.
func (r *Requests) Verification(ctx context.Context, typed, browser string) error {
	email := strings.TrimSpace(typed)
	if email == "" {
		return nil
	}
	u, found, err := r.holder(ctx, email)
	if err != nil {
		return err
	}
	request := &events.LinkRequested{Action: events.NameUserVerificationRequested, Email: email, BrowserHash: BrowserHash(browser)}
	if found {
		request.UserID, request.Email, request.Deliver = u.ID, u.Email, u.EmailVerifiedAt == nil
	}
	r.publish(ctx, request)
	return nil
}

// EmailChange asks for the change of u's login address to newEmail (NewEmailFor), once the caller
// has proven it is u. Whether newEmail is Taken is never told to the requester: every request takes
// a slot of the address's budget and publishes exactly one event for it, the request of the
// confirm link when it is free, the claim of the address, whose owner is told without a token, when
// it is Taken; a spent budget publishes it undelivered.
func (r *Requests) EmailChange(ctx context.Context, u *ent.User, newEmail string) error {
	granted, err := r.attempts.Take(ctx, throttle.KindEmailChange, newEmail)
	if err != nil {
		return err
	}
	taken, err := r.users.EmailTaken(ctx, newEmail)
	if err != nil {
		return err
	}
	if taken {
		r.publish(ctx, &events.AddressClaimed{Email: newEmail, Deliver: granted})
	} else {
		r.publish(ctx, &events.LinkRequested{Action: events.NameUserEmailChangeRequested, UserID: u.ID, Email: newEmail, Deliver: granted})
	}
	return nil
}

// DeletionLink asks for the one-time deletion link of the User userID, who typed their address
// (DeletionSubject) and has no password (ConfirmsDeletionByLink), to that Verified address. Like
// the email change, every request takes a slot of the address's budget; a spent budget publishes
// the request undelivered.
func (r *Requests) DeletionLink(ctx context.Context, userID int64, typed string) error {
	u, err := r.users.DeletionSubject(ctx, userID, typed)
	if err != nil {
		return err
	}
	if err := ConfirmsDeletionByLink(u); err != nil {
		return err
	}
	granted, err := r.attempts.Take(ctx, throttle.KindEmailChange, u.Email)
	if err != nil {
		return err
	}
	r.publish(ctx, &events.LinkRequested{Action: events.NameUserDeletionRequested, UserID: u.ID, Email: u.Email, Deliver: granted})
	return nil
}

// holder is the User that holds email; found is false when none does.
func (r *Requests) holder(ctx context.Context, email string) (u *ent.User, found bool, err error) {
	u, err = r.users.UserByEmail(ctx, email)
	if ent.IsNotFound(err) {
		return nil, false, nil
	}
	return u, err == nil, err
}

// publish publishes request in a transaction of its own, best-effort.
func (r *Requests) publish(ctx context.Context, request events.DomainEvent) {
	if err := r.users.bus.Publish(ctx, request); err != nil {
		slog.WarnContext(ctx, "request not published", "event", request.EventName(), "error", err)
	}
}
