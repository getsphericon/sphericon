package signup

import (
	"context"
	"crypto/rand"
	"errors"
	"net/mail"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
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
			first, err := u.firstWorkspaceIn(ctx, tx, usr.ID)
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

// Verified is a User a step made Verified, and the Workspace they land in next: the one joined by
// domain, else their first.
type Verified struct {
	User    *ent.User
	Landing *ent.Workspace
}

// VerifyEmailInput is the completion of a User's Sign-up: the User, and the password hash that
// replaces the one stored at Sign-up (nil keeps it).
type VerifyEmailInput struct {
	UserID       int64
	PasswordHash *string
}

// VerifyEmailLinkInput is a followed verification link: its token, the hash of the Sign-up
// browser the request carries, and the password a request from another browser must send.
type VerifyEmailLinkInput struct {
	Token    string
	Browser  string
	Password string
}

// PasswordReset is a password set from a reset link: the User, and whether it completed their
// Sign-up (the caller then sends the welcome email).
type PasswordReset struct {
	User            *ent.User
	CompletedSignUp bool
}

// VerifyEmail completes a Sign-up (ADR 0030), in one transaction: it stamps the Verified email,
// sets the password hash to *in.PasswordHash (nil keeps the one stored at Sign-up), bumps the
// session epoch (ending every session issued before, as a password change does) and creates the
// User's first Workspace, named users.FirstWorkspaceName, with the User as owner. It records
// `user.email_verify` in that Workspace's Audit log and returns the User. The address was just
// proven, so it is a domain-join step (ADR 0051): a verified Join domain equal to its domain,
// with the switch on, makes the User a `member` of that Workspace before the first Workspace is
// created. Landing is where the User goes next: the joined Workspace, else the first one.
// users.ErrAlreadyVerified when the User is no longer Unverified (a used link).
func (u *UseCase) VerifyEmail(ctx context.Context, in VerifyEmailInput) (*Verified, error) {
	verified, landing, err := u.verified(ctx, provenNow, func(tx *ent.Client, _ events.Publisher) (*ent.User, error) {
		return users.MarkVerifiedIn(ctx, tx, in.UserID, "", in.PasswordHash)
	})
	if err != nil {
		return nil, err
	}
	return &Verified{User: verified, Landing: landing}, nil
}

// VerifyEmailLink completes a Sign-up from its verification link: the link is read first
// (users.Links.Verification, with its refusals), then VerifyEmail, whose User and landing it
// returns. A link that loses the race to another one is users.ErrVerifyLinkUsed.
func (u *UseCase) VerifyEmailLink(ctx context.Context, in VerifyEmailLinkInput) (*Verified, error) {
	uid, hash, err := u.links.Verification(ctx, in.Token, in.Browser, in.Password)
	if err != nil {
		return nil, err
	}
	verified, err := u.VerifyEmail(ctx, VerifyEmailInput{UserID: uid, PasswordHash: hash})
	if errors.Is(err, users.ErrAlreadyVerified) {
		return nil, users.ErrVerifyLinkUsed
	}
	return verified, err
}

// ResetPasswordLink sets the User's password from a reset link (ADR 0030): the password rule and
// the link first (users.Links.PasswordReset), then the reset. The link was mailed to the login
// address, so following it proves the address as much as the verification link does: for an
// Unverified User it completes the Sign-up exactly as VerifyEmail (Verified email, first
// Workspace, `user.email_verify`) and reports CompletedSignUp with the User, so the caller
// sends the welcome email. For a Verified User it is only a password change, recorded like
// users.Module.ChangeProfile. Either way every earlier session ends.
func (u *UseCase) ResetPasswordLink(ctx context.Context, token, password string) (*PasswordReset, error) {
	id, passwordHash, err := u.links.PasswordReset(ctx, token, password)
	if err != nil {
		return nil, err
	}
	stored, err := u.users.User(ctx, id)
	if err != nil {
		return nil, err
	}
	if stored.EmailVerifiedAt == nil {
		verified, err := u.VerifyEmail(ctx, VerifyEmailInput{UserID: id, PasswordHash: &passwordHash})
		if err == nil {
			return &PasswordReset{User: verified.User, CompletedSignUp: true}, nil
		}
		if !errors.Is(err, users.ErrAlreadyVerified) {
			return nil, err
		}
		// A concurrent link verified the User first: this is now a plain password change.
	}
	reset, err := u.users.ResetVerifiedPassword(ctx, id, passwordHash)
	if err != nil {
		return nil, err
	}
	return &PasswordReset{User: reset}, nil
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

// ErrOwnerEmailInvalid refuses an Owner made from a shell whose address does not parse.
var ErrOwnerEmailInvalid = validation.RefuseField(validation.FieldEmail, "signup: the owner's address is not an email address", i18n.ErrorsEmailInvalid)

// CreateOwner is the Instance operator's way to make an Owner from a shell (ADR 0042): a
// Verified User with a generated password and its first Workspace, named
// users.FirstWorkspaceName, exactly as SignUpVerified makes them. The password is returned once;
// an address that does not parse is refused, and the unique index on the email refuses a taken
// one (see db.IsUniqueViolation).
func (u *UseCase) CreateOwner(ctx context.Context, email, name string) (string, error) {
	if _, err := mail.ParseAddress(email); err != nil {
		return "", ErrOwnerEmailInvalid.With(email)
	}
	password := rand.Text()
	hash, err := u.passwords.Hash(password)
	if err != nil {
		return "", err
	}
	if _, err := u.SignUpVerified(ctx, users.NewUser{Name: name, Email: email, PasswordHash: hash}); err != nil {
		return "", err
	}
	return password, nil
}
