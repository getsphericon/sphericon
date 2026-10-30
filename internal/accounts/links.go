package accounts

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/signup"
	"github.com/getsphericon/sphericon/internal/validation"
)

// How long each mailed link works.
const (
	resetLinkTTL       = time.Hour
	emailChangeLinkTTL = time.Hour
	deletionLinkTTL    = time.Hour
	notMeLinkTTL       = 14 * 24 * time.Hour
)

// VerificationLinkTTL is how long an email verification link works; the Sign-up browser's
// cookie lives as long, so the link opened in that browser keeps the password it was signed up
// with.
const VerificationLinkTTL = 24 * time.Hour

// The refusals of a mailed link that does not stand (KindLinkInvalid). A link whose binding
// lookup fails on storage is not one of them: that is the storage's error.
var (
	// ErrResetLinkInvalid is a password reset link that is forged, expired or spent.
	ErrResetLinkInvalid = validation.LinkInvalid("accounts: reset link invalid or expired", "errors.reset_link_invalid")
	// ErrVerifyLinkInvalid is an email verification link that is forged, expired, or whose
	// password changed since.
	ErrVerifyLinkInvalid = validation.LinkInvalid("accounts: verification link invalid or expired", "errors.verify_link_invalid")
	// ErrVerifyLinkUsed is an email verification link of a User that is gone or already
	// Verified.
	ErrVerifyLinkUsed = validation.LinkInvalid("accounts: verification link already used", "errors.verify_link_bad")
	// ErrVerifyLinkStale is an email verification link sent to an address the User no longer
	// has: an old link never verifies a changed address.
	ErrVerifyLinkStale = validation.LinkInvalid("accounts: verification link of an earlier address", "errors.verify_link_stale")
	// ErrConfirmLinkInvalid is an email change link that is forged, expired or spent, or whose
	// new address was Taken meanwhile: to the confirmer a Taken address reads as an invalid link
	// (ADR 0031). It is also a User deletion link that is expired, used or another User's.
	ErrConfirmLinkInvalid = validation.LinkInvalid("accounts: confirmation link invalid or expired", "errors.confirm_link_invalid")
	// ErrConfirmLinkBad is an email change link that names no new address.
	ErrConfirmLinkBad = validation.LinkInvalid("accounts: confirmation link names no address", "errors.confirm_link_bad")
	// ErrNotMeLinkInvalid is a "this was not me" link that is forged or expired, or whose
	// Linked identity is already gone.
	ErrNotMeLinkInvalid = validation.LinkInvalid("accounts: not-me link invalid or used", "errors.not_me_link_invalid")
)

// ErrVerifyPasswordNeeded is a verification link opened outside the Sign-up browser without a
// password: that browser must set one, which replaces the one stored at Sign-up, and nothing
// changes until it does (the link stays valid, so a mail scanner consumes nothing).
var ErrVerifyPasswordNeeded = validation.Incomplete("accounts: a password is needed outside the sign-up browser", "errors.verify_password_needed")

// The claims a link carries beyond its User.
const (
	// claimEmail is the address a verification link was sent to.
	claimEmail = "email"
	// claimBrowser is the hash of the Sign-up browser a verification link trusts.
	claimBrowser = "sign_up_browser"
	// claimNewEmail is the address an email change link asks for.
	claimNewEmail = "new"
	// claimIdentity names the Linked identity a "this was not me" link undoes.
	claimIdentity = "identity"
)

// Links mints and reads the links the self-service account flows mail (password reset, email
// verification, email change, User deletion, "this was not me"). Each is an authtoken of its
// purpose, keyed to a value that changes once its action lands, so it works once with no
// store. A link that does not stand is refused with its flow's sentinel; deciding that is
// this type's, so no handler parses a token.
type Links struct {
	accounts *Accounts
	tokens   *authtoken.Signer
}

// NewLinks builds the links of a over the signer of the self-service tokens.
func NewLinks(a *Accounts, tokens *authtoken.Signer) *Links {
	return &Links{accounts: a, tokens: tokens}
}

// passwordForm is a new password a link sets, checked against the password rule.
type passwordForm struct {
	Password string `json:"password" validate:"required,password_not_blank,password_length"`
}

// hashPassword checks a new password against the password rule and hashes it.
func (l *Links) hashPassword(password string) (string, error) {
	if err := validation.Form(passwordForm{Password: password}); err != nil {
		return "", err
	}
	return l.accounts.passwords.Hash(password)
}

// lookupFailed reports whether a token parse failed because storage could not be read, as
// opposed to the token being unusable: a User that is gone makes the link stale (refused), a
// failing database is the storage's error.
func lookupFailed(err error) bool {
	lookup, ok := errors.AsType[*authtoken.BindingError](err)
	return ok && !ent.IsNotFound(lookup.Err)
}

// parse reads a link of purpose bound by binding, refused with invalid when it does not stand.
func (l *Links) parse(token string, purpose authtoken.Purpose, binding func(int64) (string, error), invalid error) (userID int64, extra map[string]string, err error) {
	userID, extra, err = l.tokens.Parse(token, purpose, binding)
	if lookupFailed(err) {
		return 0, nil, err
	}
	if err != nil {
		return 0, nil, invalid
	}
	return userID, extra, nil
}

// passwordHashOf is the binding of a reset or verification link: the User's password hash.
func (l *Links) passwordHashOf(ctx context.Context) func(int64) (string, error) {
	return func(id int64) (string, error) {
		u, err := l.accounts.User(ctx, id)
		if err != nil {
			return "", err
		}
		return u.PasswordHash, nil
	}
}

// PasswordResetLink mints the token of the password reset link mailed to u, bound to the
// current password hash: once the password is reset the hash changes and the link stops
// working. A nil u (an address that holds no User) gets a throwaway token over the same work.
func (l *Links) PasswordResetLink(u *ent.User) (string, error) {
	var uid int64
	var binding string
	if u != nil {
		uid, binding = u.ID, u.PasswordHash
	}
	return l.tokens.Mint(authtoken.PurposePasswordReset, uid, binding, resetLinkTTL, nil)
}

// ResetPassword sets password from a reset link: the password rule first (a refused one costs
// no hash), then the link (ErrResetLinkInvalid), then Accounts.ResetPassword, whose results it
// returns.
func (l *Links) ResetPassword(ctx context.Context, token, password string) (*ent.User, bool, error) {
	if err := validation.Form(passwordForm{Password: password}); err != nil {
		return nil, false, err
	}
	uid, _, err := l.parse(token, authtoken.PurposePasswordReset, l.passwordHashOf(ctx), ErrResetLinkInvalid)
	if err != nil {
		return nil, false, err
	}
	hash, err := l.accounts.passwords.Hash(password)
	if err != nil {
		return nil, false, err
	}
	return l.accounts.ResetPassword(ctx, uid, hash)
}

// VerificationLink mints the token of the email verification link mailed to u (ADR 0030),
// bound to the password stored at Sign-up and carrying the address it is sent to and
// browserHash, the hash of the Sign-up browser it trusts (empty for none). A nil u (an address
// that holds no User) gets a throwaway token over the same work.
func (l *Links) VerificationLink(u *ent.User, browserHash string) (string, error) {
	var uid int64
	var binding, email string
	if u != nil {
		uid, binding, email = u.ID, u.PasswordHash, u.Email
	}
	return l.tokens.Mint(authtoken.PurposeEmailVerify, uid, binding, VerificationLinkTTL,
		map[string]string{claimEmail: email, claimBrowser: browserHash})
}

// VerifyEmail completes a Sign-up from its verification link (Accounts.VerifyEmail) and
// returns the Verified User and the Workspace to land in. browser is the Sign-up browser
// cookie the request carries, "" for none: the Sign-up browser keeps the password stored at
// Sign-up, and any other browser must send password, which replaces it. The link is single use
// and scoped to the address it was sent to. Refusals: ErrVerifyLinkInvalid, ErrVerifyLinkUsed,
// ErrVerifyLinkStale, ErrVerifyPasswordNeeded, and a *validation.FieldsError for a password
// the rule refuses (the link stays valid for a better one).
func (l *Links) VerifyEmail(ctx context.Context, token, browser, password string) (*ent.User, *ent.Workspace, error) {
	uid, extra, err := l.parse(token, authtoken.PurposeEmailVerify, l.passwordHashOf(ctx), ErrVerifyLinkInvalid)
	if err != nil {
		return nil, nil, err
	}
	u, err := l.accounts.User(ctx, uid)
	if ent.IsNotFound(err) {
		return nil, nil, ErrVerifyLinkUsed
	}
	if err != nil {
		return nil, nil, err
	}
	if u.EmailVerifiedAt != nil {
		return nil, nil, ErrVerifyLinkUsed
	}
	if extra[claimEmail] != u.Email {
		return nil, nil, ErrVerifyLinkStale
	}
	var hash *string
	if !signup.SameBrowser(extra[claimBrowser], browser) {
		if password == "" {
			return nil, nil, ErrVerifyPasswordNeeded
		}
		replacement, err := l.hashPassword(password)
		if err != nil {
			return nil, nil, err
		}
		hash = &replacement
	}
	verified, landing, err := l.accounts.VerifyEmail(ctx, uid, hash)
	if errors.Is(err, ErrAlreadyVerified) {
		return nil, nil, ErrVerifyLinkUsed
	}
	return verified, landing, err
}

// EmailChange is what an email change link asks: the User and the address to change to.
type EmailChange struct {
	UserID   int64
	NewEmail string
}

// EmailChangeLink mints the token of the link mailed to newEmail to confirm u's change to it,
// bound to the current email so it works once (invalid once the email changed).
func (l *Links) EmailChangeLink(u *ent.User, newEmail string) (string, error) {
	return l.tokens.Mint(authtoken.PurposeEmailChange, u.ID, u.Email, emailChangeLinkTTL, map[string]string{claimNewEmail: newEmail})
}

// EmailChange reads an email change link: ErrConfirmLinkInvalid when it does not stand,
// ErrConfirmLinkBad when it names no address.
func (l *Links) EmailChange(ctx context.Context, token string) (EmailChange, error) {
	uid, extra, err := l.parse(token, authtoken.PurposeEmailChange, func(id int64) (string, error) {
		u, err := l.accounts.User(ctx, id)
		if err != nil {
			return "", err
		}
		return u.Email, nil
	}, ErrConfirmLinkInvalid)
	if err != nil {
		return EmailChange{}, err
	}
	newEmail := strings.TrimSpace(extra[claimNewEmail])
	if newEmail == "" {
		return EmailChange{}, ErrConfirmLinkBad
	}
	return EmailChange{UserID: uid, NewEmail: newEmail}, nil
}

// ConfirmEmailChange applies the change a link asked (Accounts.ChangeEmail). The new address
// was proven by following the link, so it is Verified. An address Taken meanwhile (the unique
// index guards the race) is ErrConfirmLinkInvalid, never "in use" (ADR 0031).
func (l *Links) ConfirmEmailChange(ctx context.Context, c EmailChange) error {
	err := l.accounts.ChangeEmail(ctx, c.UserID, c.NewEmail)
	if db.IsUniqueViolation(err) {
		return ErrConfirmLinkInvalid
	}
	return err
}

// deletionBinding keys a deletion link to the User's session epoch, so signing out everywhere
// (or any other epoch bump) kills a link already mailed.
func deletionBinding(u *ent.User) string { return strconv.FormatInt(u.SessionEpoch, 10) }

// DeletionLink mints the token of the one-time link that confirms the deletion of u, a User
// with no password (ADR 0050); a User with a password confirms with it instead
// (ErrDeletionUsePassword). It is bound to the User and their session epoch, never to the
// address, and works once: the deletion removes the User it names.
func (l *Links) DeletionLink(u *ent.User) (string, error) {
	if u.PasswordHash != "" {
		return "", refuseDeletion(ErrDeletionUsePassword)
	}
	return l.tokens.Mint(authtoken.PurposeUserDeletion, u.ID, deletionBinding(u), deletionLinkTTL, nil)
}

// CheckDeletionLink refuses with ErrConfirmLinkInvalid a token that is not a live deletion link
// issued to u: expired, used, or another User's (a new User on the same address).
func (l *Links) CheckDeletionLink(token string, u *ent.User) error {
	uid, _, err := l.tokens.Parse(token, authtoken.PurposeUserDeletion, func(id int64) (string, error) {
		if id != u.ID {
			return "", errors.New("deletion link of another user")
		}
		return deletionBinding(u), nil
	})
	if err != nil || uid != u.ID {
		return ErrConfirmLinkInvalid
	}
	return nil
}

// NotMeLink mints the token of the "this was not me" link of the notice mailed when a Sign-in
// provider was linked to u silently (ADR 0048): it names the Linked identity to undo and has
// no binding, since once the identity is gone the link has nothing left to do.
func (l *Links) NotMeLink(u *ent.User, identityID int64) (string, error) {
	return l.tokens.Mint(authtoken.PurposeIdentityNotMe, u.ID, "", notMeLinkTTL,
		map[string]string{claimIdentity: strconv.FormatInt(identityID, 10)})
}

// UndoNotMe follows a "this was not me" link: it unlinks the identity the link names and ends
// every session of its User (Accounts.UndoLink). ErrNotMeLinkInvalid when the link does not
// stand or the identity is already gone.
func (l *Links) UndoNotMe(ctx context.Context, token string) error {
	uid, extra, err := l.tokens.Parse(token, authtoken.PurposeIdentityNotMe, func(int64) (string, error) { return "", nil })
	if err != nil {
		return ErrNotMeLinkInvalid
	}
	identityID, err := strconv.ParseInt(extra[claimIdentity], 10, 64)
	if err != nil {
		return ErrNotMeLinkInvalid
	}
	removed, err := l.accounts.UndoLink(ctx, uid, identityID)
	if err != nil {
		return err
	}
	if !removed {
		return ErrNotMeLinkInvalid
	}
	return nil
}
