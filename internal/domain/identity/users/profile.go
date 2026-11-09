package users

import (
	"context"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/signupitems"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// ProfileChange is what a User asks to change on their own profile. A nil Name keeps the name;
// a nil Timezone keeps the User time zone (ADR 0068) unless ClearTimezone clears it, and a nil
// RegionalFormat keeps the Regional format unless ClearRegionalFormat clears it; an empty
// NewPassword keeps the password, and then CurrentPassword is not read. The email is the login
// identity and changes through its own confirmed link, not here.
type ProfileChange struct {
	Name           *string
	Timezone       *string
	ClearTimezone  bool
	RegionalFormat *string
	// ClearRegionalFormat clears the Regional format; a RegionalFormat given with it wins.
	ClearRegionalFormat bool
	NewPassword         string
	CurrentPassword     string
}

// Prover proves a User's current password the way a login does (ADR 0025): the login module's one
// prove path (login.Module.ProvePassword), its throttle first, the decoy hash, a failure counted on
// the Login budget. It returns nil when password is the account's and its refusal otherwise (a
// wrong password, or, while the throttle's delay runs, the delay even for a right password). The
// login module sits above this one, so the composition root hands it in (WithProver).
type Prover interface {
	ProvePassword(ctx context.Context, address, passwordHash, password string) error
}

// WithProver gives the module the proof of a User's current password, which every step that asks
// for it takes (ProveIdentity, ChangeProfile, Requests.EmailChange).
func WithProver(p Prover) Option {
	return func(a *Module) { a.prover = p }
}

// provePassword proves that password is u's current one (Prover).
func (a *Module) provePassword(ctx context.Context, u *ent.User, password string) error {
	return a.prover.ProvePassword(ctx, u.Email, u.PasswordHash, password)
}

// ErrIdentityProofRequired is a step that asks for the password, of a User who has none and
// whose session holds no fresh proof instead (ADR 0048): a Passkey check or a provider login.
var ErrIdentityProofRequired = validation.Denied("users: a fresh identity proof is required", i18n.ErrorsIdentityProofRequired)

// identityProofTTL is how long a Sign-in provider login (ADR 0048) or a Passkey check (ADR 0052)
// stands in for the password of a User who has none: the proof is the time carried by the
// session, so it needs no store and ends by itself.
const identityProofTTL = 5 * time.Minute

// IdentityProof is what a signed-in User offers for a step that asks for the password (ADR 0020,
// ADR 0048): the password typed, and when their session last held a Sign-in provider login and a
// Passkey check (zero for none), either of which stands in for it while recent enough.
type IdentityProof struct {
	Password        string
	ProviderLoginAt time.Time
	PasskeyCheckAt  time.Time
}

// FreshAt is whether the session holds, at now, a Sign-in provider login or a Passkey check of
// the last identityProofTTL. A time ahead of now is no proof.
func (p IdentityProof) FreshAt(now time.Time) bool {
	recent := func(at time.Time) bool {
		if at.IsZero() {
			return false
		}
		age := now.Sub(at)
		return age >= 0 && age <= identityProofTTL
	}
	return recent(p.ProviderLoginAt) || recent(p.PasskeyCheckAt)
}

// StandsInFor is whether the proof, at now, stands in for the password of u: u has none and the
// session's proof is fresh (FreshAt). A User with a password proves it by typing it.
func (p IdentityProof) StandsInFor(u *ent.User, now time.Time) bool {
	return u.PasswordHash == "" && p.FreshAt(now)
}

// ProveIdentity is the proof the steps that ask for the password need (ADR 0020, ADR 0048). A
// User with a password proves it the way a login does (Prover, ADR 0025): while the account's
// Login throttle delay runs it fails even for the right password, a wrong password counts as a
// failure of the account, and a right one does not reset the counter (only a completed login
// does, else a session holder could clear a run of second-step guesses). A User with no password
// proves instead a Sign-in provider login or a Passkey check recent enough (IdentityProof.FreshAt
// on the module's clock), carried by the session; there is nothing to guess, so nothing is counted, and without
// one the step is refused with ErrIdentityProofRequired. A step proves before its transaction,
// so a refused proof is counted whatever the step would have decided.
func (a *Module) ProveIdentity(ctx context.Context, u *ent.User, proof IdentityProof) error {
	if u.PasswordHash != "" {
		return a.provePassword(ctx, u, proof.Password)
	}
	if proof.FreshAt(a.now()) {
		return nil
	}
	return ErrIdentityProofRequired
}

// ErrNewEmailSame refuses an email change to the address the User already has.
var ErrNewEmailSame = validation.RefuseField("newEmail", "users: the new email is the current one", i18n.ErrorsNewEmailMustDiffer)

// newEmailForm is the address an email change asks for, checked against the address rule.
type newEmailForm struct {
	NewEmail string `json:"newEmail" validate:"required,account_email"`
}

// NewEmailFor is the address the User u asks to change their login email to, typed as it was:
// trimmed, refused as a *validation.FieldsError when it is no address, and ErrNewEmailSame when
// it is the User's own.
func NewEmailFor(u *ent.User, typed string) (string, error) {
	email := strings.TrimSpace(typed)
	if err := validation.Form(newEmailForm{NewEmail: email}); err != nil {
		return "", err
	}
	if strings.EqualFold(email, u.Email) {
		return "", ErrNewEmailSame
	}
	return email, nil
}

// ProfileChanged is what a profile change left.
type ProfileChanged struct {
	User *ent.User
	// Pending is the required acceptances pending for the User after the change (ADR 0062), which
	// their profile shows.
	Pending []signupitems.Document
	// PasswordChanged reports that the change ended every session of the User (ADR 0020): the
	// acting session continues only under a fresh one, stamped with the new epoch.
	PasswordChanged bool
}

// newPasswordForm is the new password of a profile change, checked against the password rule.
type newPasswordForm struct {
	NewPassword string `json:"newPassword" validate:"password_not_blank,password_length"`
}

// ChangeProfile changes the name, the time zone, the Regional format and/or the password of the User
// u. A blank name is refused, and so is a time zone the zone database does not know
// (ErrUnknownZone) and a Regional format that is no well-formed tag (ErrMalformedRegionalFormat); a new
// password is checked against the password rule before the current one is proven, so a
// refused one costs no hash, and it needs the current password, proven through the Prover. A
// refused field is a *validation.FieldsError, and the Prover's refusal (a wrong current password,
// a running throttle delay) comes back as it is. Nothing is written unless every rule holds; a
// change that asks for nothing writes nothing and returns u. Either way it returns the required
// acceptances pending for the User, which their profile shows.
func (a *Module) ChangeProfile(ctx context.Context, u *ent.User, c ProfileChange) (ProfileChanged, error) {
	changed, err := a.changeProfile(ctx, u, c)
	if err != nil {
		return ProfileChanged{}, err
	}
	if changed.Pending, err = a.consents.Pending(ctx, u.ID); err != nil {
		return ProfileChanged{}, err
	}
	return changed, nil
}

func (a *Module) changeProfile(ctx context.Context, u *ent.User, c ProfileChange) (ProfileChanged, error) {
	w := profileWrite{
		Timezone:            c.Timezone,
		ClearTimezone:       c.ClearTimezone && c.Timezone == nil,
		ClearRegionalFormat: c.ClearRegionalFormat && c.RegionalFormat == nil,
	}
	if c.Name != nil {
		trimmed := strings.TrimSpace(*c.Name)
		if trimmed == "" {
			return ProfileChanged{}, validation.Refused("name", i18n.T(i18n.ErrorsNameEmpty, nil))
		}
		if err := CheckName(trimmed); err != nil {
			return ProfileChanged{}, err
		}
		w.Name = &trimmed
	}
	if c.Timezone != nil {
		if err := workspaces.CheckZone(*c.Timezone); err != nil {
			return ProfileChanged{}, err
		}
	}
	if c.RegionalFormat != nil {
		tag, err := canonicalRegionalFormat(*c.RegionalFormat)
		if err != nil {
			return ProfileChanged{}, err
		}
		w.RegionalFormat = &tag
	}
	if c.NewPassword != "" {
		if err := validation.Form(newPasswordForm{NewPassword: c.NewPassword}); err != nil {
			return ProfileChanged{}, err
		}
		if c.CurrentPassword == "" {
			return ProfileChanged{}, validation.Refused("currentPassword", i18n.T(i18n.ErrorsCurrentPasswordRequired, nil))
		}
		if err := a.provePassword(ctx, u, c.CurrentPassword); err != nil {
			return ProfileChanged{}, err
		}
		h, err := a.passwords.Hash(c.NewPassword)
		if err != nil {
			return ProfileChanged{}, err
		}
		w.PasswordHash = &h
	}
	if w == (profileWrite{}) {
		return ProfileChanged{User: u}, nil
	}
	saved, err := a.updateProfile(ctx, u.ID, w, false)
	if err != nil {
		return ProfileChanged{}, err
	}
	return ProfileChanged{User: saved, PasswordChanged: w.PasswordHash != nil}, nil
}
