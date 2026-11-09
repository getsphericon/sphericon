package users

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/kernel/authtoken"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
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
	ErrResetLinkInvalid = validation.LinkInvalid("users: reset link invalid or expired", i18n.ErrorsResetLinkInvalid)
	// ErrVerifyLinkInvalid is an email verification link that is forged, expired, or whose
	// password changed since.
	ErrVerifyLinkInvalid = validation.LinkInvalid("users: verification link invalid or expired", i18n.ErrorsVerifyLinkInvalid)
	// ErrVerifyLinkUsed is an email verification link of a User that is gone or already
	// Verified.
	ErrVerifyLinkUsed = validation.LinkInvalid("users: verification link already used", i18n.ErrorsVerifyLinkBad)
	// ErrVerifyLinkStale is an email verification link sent to an address the User no longer
	// has: an old link never verifies a changed address.
	ErrVerifyLinkStale = validation.LinkInvalid("users: verification link of an earlier address", i18n.ErrorsVerifyLinkStale)
	// ErrConfirmLinkInvalid is an email change link that is forged, expired or spent, or whose
	// new address was Taken meanwhile: to the confirmer a Taken address reads as an invalid link
	// (ADR 0031). It is also a User deletion link that is expired, used or another User's.
	ErrConfirmLinkInvalid = validation.LinkInvalid("users: confirmation link invalid or expired", i18n.ErrorsConfirmLinkInvalid)
	// ErrConfirmLinkBad is an email change link that names no new address.
	ErrConfirmLinkBad = validation.LinkInvalid("users: confirmation link names no address", i18n.ErrorsConfirmLinkBad)
	// ErrNotMeLinkInvalid is a "this was not me" link that is forged or expired, or whose
	// Linked identity is already gone.
	ErrNotMeLinkInvalid = validation.LinkInvalid("users: not-me link invalid or used", i18n.ErrorsNotMeLinkInvalid)
)

// ErrVerifyPasswordNeeded is a verification link opened outside the Sign-up browser without a
// password: that browser must set one, which replaces the one stored at Sign-up, and nothing
// changes until it does (the link stays valid, so a mail scanner consumes nothing).
var ErrVerifyPasswordNeeded = validation.Incomplete("users: a password is needed outside the sign-up browser", i18n.ErrorsVerifyPasswordNeeded)

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
	users  *Module
	tokens *authtoken.Signer
}

// NewLinks builds the links of a over the signer of the self-service tokens.
func NewLinks(a *Module, tokens *authtoken.Signer) *Links {
	return &Links{users: a, tokens: tokens}
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
	return l.users.passwords.Hash(password)
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
		u, err := l.users.User(ctx, id)
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

// PasswordReset reads a reset link and the new password it sets: the password rule first (a
// refused one costs no hash), then the link (ErrResetLinkInvalid). It returns the User the link
// names and the new password's hash, for the reset (internal/usecase/signup, which
// completes the Sign-up of an Unverified User).
func (l *Links) PasswordReset(ctx context.Context, token, password string) (userID int64, passwordHash string, err error) {
	if err := validation.Form(passwordForm{Password: password}); err != nil {
		return 0, "", err
	}
	uid, _, err := l.parse(token, authtoken.PurposePasswordReset, l.passwordHashOf(ctx), ErrResetLinkInvalid)
	if err != nil {
		return 0, "", err
	}
	hash, err := l.users.passwords.Hash(password)
	if err != nil {
		return 0, "", err
	}
	return uid, hash, nil
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

// Verification reads a verification link (ADR 0030) for the completion of its Sign-up
// (internal/usecase/signup): the User it names and the password hash to set, nil to
// keep the one stored at Sign-up. browser is the Sign-up browser cookie the request carries, ""
// for none: the Sign-up browser keeps the password stored at Sign-up, and any other browser must
// send password, which replaces it. The link is single use and scoped to the address it was sent
// to. Refusals: ErrVerifyLinkInvalid, ErrVerifyLinkUsed, ErrVerifyLinkStale,
// ErrVerifyPasswordNeeded, and a *validation.FieldsError for a password the rule refuses (the
// link stays valid for a better one).
func (l *Links) Verification(ctx context.Context, token, browser, password string) (userID int64, passwordHash *string, err error) {
	uid, extra, err := l.parse(token, authtoken.PurposeEmailVerify, l.passwordHashOf(ctx), ErrVerifyLinkInvalid)
	if err != nil {
		return 0, nil, err
	}
	u, err := l.users.User(ctx, uid)
	if ent.IsNotFound(err) {
		return 0, nil, ErrVerifyLinkUsed
	}
	if err != nil {
		return 0, nil, err
	}
	if u.EmailVerifiedAt != nil {
		return 0, nil, ErrVerifyLinkUsed
	}
	if extra[claimEmail] != u.Email {
		return 0, nil, ErrVerifyLinkStale
	}
	if SameBrowser(extra[claimBrowser], browser) {
		return uid, nil, nil
	}
	if password == "" {
		return 0, nil, ErrVerifyPasswordNeeded
	}
	replacement, err := l.hashPassword(password)
	if err != nil {
		return 0, nil, err
	}
	return uid, &replacement, nil
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
		u, err := l.users.User(ctx, id)
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

// ConfirmEmailChange applies the change a link asked (Module.ChangeEmail). The new address
// was proven by following the link, so it is Verified. An address Taken meanwhile (the unique
// index guards the race) is ErrConfirmLinkInvalid, never "in use" (ADR 0031).
func (l *Links) ConfirmEmailChange(ctx context.Context, c EmailChange) error {
	err := l.users.ChangeEmail(ctx, c.UserID, c.NewEmail)
	if errors.Is(err, ErrEmailTaken) {
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
	if err := ConfirmsDeletionByLink(u); err != nil {
		return "", err
	}
	return l.tokens.Mint(authtoken.PurposeUserDeletion, u.ID, deletionBinding(u), deletionLinkTTL, nil)
}

// ConfirmsDeletionByLink refuses with ErrDeletionUsePassword a User who has a password: only a
// User with none confirms their deletion with a mailed link (ADR 0050).
func ConfirmsDeletionByLink(u *ent.User) error {
	if u.PasswordHash != "" {
		return refuseDeletion(ErrDeletionUsePassword)
	}
	return nil
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
// every session of its User (Module.UndoLink). ErrNotMeLinkInvalid when the link does not
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
	removed, err := l.users.UndoLink(ctx, uid, identityID)
	if err != nil {
		return err
	}
	if !removed {
		return ErrNotMeLinkInvalid
	}
	return nil
}
