package workspacecreation

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/mail"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/users"
)

// provenNow and noJoin say whether a step that makes a User Verified also proved the address
// (stamped email_proven_at from empty), which makes it a domain-join step (ADR 0051). An
// Instance with verification off stamps only verified_at, which proves nothing.
const (
	provenNow = true
	noJoin    = false
)

// verified is what makes a User Verified, shared by VerifyEmail, SignUpVerified and the
// provider sign-in: write stores the User's verified state (and whatever else its step writes,
// on the same publisher); then, when proven (the step stamped email_proven_at from empty), the
// domain-join step runs; then the User's first Workspace is created and `user.email_verify`
// recorded in its Audit log, all in one transaction. landing is the Workspace the User goes to
// next: the one joined by domain, else the first.
func (u *UseCase) verified(ctx context.Context, proven bool, write func(tx *ent.Client, pub events.Publisher) (*ent.User, error)) (*ent.User, *ent.Workspace, error) {
	for {
		var verified *ent.User
		var landing *ent.Workspace
		written := false
		err := u.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
			usr, err := write(tx, pub)
			if err != nil {
				return err
			}
			written = true
			var joined *ent.Workspace
			if proven {
				if joined, err = users.ApplyDomainJoinIn(ctx, tx, pub, usr); err != nil {
					return err
				}
			}
			first, err := u.createIn(ctx, tx, usr.ID, users.FirstWorkspaceName)
			if err != nil {
				return err
			}
			verified, landing = usr, first
			if joined != nil {
				landing = joined
			}
			return users.RecordUserAction(ctx, tx, pub, usr, events.ActionUserEmailVerify, nil)
		})
		if written && db.IsUniqueViolation(err) {
			continue // drew a taken id; the next attempt retries
		}
		return verified, landing, err
	}
}

// VerifyEmail completes a Sign-up (ADR 0030), in one transaction: it stamps the Verified email,
// sets the password hash to *passwordHash (nil keeps the one stored at Sign-up), bumps the
// session epoch (ending every session issued before, as a password change does) and creates the
// User's first Workspace, named users.FirstWorkspaceName, with the User as owner. It records
// `user.email_verify` in that Workspace's Audit log and returns the User. The address was just
// proven, so it is a domain-join step (ADR 0051): a verified Join domain equal to its domain,
// with the switch on, makes the User a `member` of that Workspace before the first Workspace is
// created. landing is where the User goes next: the joined Workspace, else the first one.
// users.ErrAlreadyVerified when the User is no longer Unverified (a used link).
func (u *UseCase) VerifyEmail(ctx context.Context, id int64, passwordHash *string) (verified *ent.User, landing *ent.Workspace, err error) {
	return u.verified(ctx, provenNow, func(tx *ent.Client, _ events.Publisher) (*ent.User, error) {
		return users.MarkVerifiedIn(ctx, tx, id, "", passwordHash)
	})
}

// VerifyEmailLink completes a Sign-up from its verification link: the link is read first
// (users.Links.Verification, with its refusals), then VerifyEmail, whose User and landing it
// returns. A link that loses the race to another one is users.ErrVerifyLinkUsed.
func (u *UseCase) VerifyEmailLink(ctx context.Context, token, browser, password string) (*ent.User, *ent.Workspace, error) {
	uid, hash, err := u.links.Verification(ctx, token, browser, password)
	if err != nil {
		return nil, nil, err
	}
	verified, landing, err := u.VerifyEmail(ctx, uid, hash)
	if errors.Is(err, users.ErrAlreadyVerified) {
		return nil, nil, users.ErrVerifyLinkUsed
	}
	return verified, landing, err
}

// ResetPasswordLink sets the User's password from a reset link (ADR 0030): the password rule and
// the link first (users.Links.PasswordReset), then the reset. The link was mailed to the login
// address, so following it proves the address as much as the verification link does: for an
// Unverified User it completes the Sign-up exactly as VerifyEmail (Verified email, first
// Workspace, `user.email_verify`) and reports completedSignUp = true with the User, so the caller
// sends the welcome email. For a Verified User it is only a password change, recorded like
// users.Module.ChangeProfile. Either way every earlier session ends.
func (u *UseCase) ResetPasswordLink(ctx context.Context, token, password string) (reset *ent.User, completedSignUp bool, err error) {
	id, passwordHash, err := u.links.PasswordReset(ctx, token, password)
	if err != nil {
		return nil, false, err
	}
	reset, err = u.users.User(ctx, id)
	if err != nil {
		return nil, false, err
	}
	if reset.EmailVerifiedAt == nil {
		reset, _, err = u.VerifyEmail(ctx, id, &passwordHash)
		if err == nil {
			return reset, true, nil
		}
		if !errors.Is(err, users.ErrAlreadyVerified) {
			return nil, false, err
		}
		// A concurrent link verified the User first: this is now a plain password change.
	}
	reset, err = u.users.ResetVerifiedPassword(ctx, id, passwordHash)
	return reset, false, err
}

// SignUpVerified is a Sign-up on an install that does not require email verification (ADR
// 0030): in one transaction it creates a User that is Verified at once with its password and the
// Consent records of its answers (ADR 0062), and the first Workspace (users.FirstWorkspaceName),
// exactly as VerifyEmail does. The unique index on the email refuses a taken address (see
// db.IsUniqueViolation). SurveyDue marks the User as owing the Sign-up survey.
func (u *UseCase) SignUpVerified(ctx context.Context, n users.NewUser) (*ent.User, error) {
	if err := users.CheckName(n.Name); err != nil {
		return nil, err
	}
	created, _, err := u.verified(ctx, noJoin, func(tx *ent.Client, _ events.Publisher) (*ent.User, error) {
		return users.CreateVerifiedIn(ctx, tx, n)
	})
	return created, err
}

// CreateOwner is the Instance operator's way to make an Owner from a shell (ADR 0042): a
// Verified User with a generated password and its first Workspace, named
// users.FirstWorkspaceName, exactly as SignUpVerified makes them. The password is returned once;
// an address that does not parse is refused, and the unique index on the email refuses a taken
// one (see db.IsUniqueViolation).
func (u *UseCase) CreateOwner(ctx context.Context, email, name string) (password string, err error) {
	if _, err := mail.ParseAddress(email); err != nil {
		return "", fmt.Errorf("%q is not an email address", email)
	}
	password = rand.Text()
	hash, err := u.passwords.Hash(password)
	if err != nil {
		return "", err
	}
	if _, err := u.SignUpVerified(ctx, users.NewUser{Name: name, Email: email, PasswordHash: hash}); err != nil {
		return "", err
	}
	return password, nil
}
