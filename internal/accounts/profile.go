package accounts

import (
	"context"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/validation"
)

// ProfileChange is what a User asks to change on their own profile. A nil Name keeps the name;
// an empty NewPassword keeps the password, and then CurrentPassword is not read. The email is
// the login identity and changes through its own confirmed link, not here.
type ProfileChange struct {
	Name            *string
	NewPassword     string
	CurrentPassword string
}

// Prove is the proof of a User's current password, which the adapter supplies: the login
// module's one prove path (its throttle first, the decoy hash, a failure counted on the Login
// budget). It returns nil when password is the User's and its refusal otherwise (a wrong
// password, or, while the throttle's delay runs, the delay even for a right password).
type Prove func(ctx context.Context, u *ent.User, password string) error

// ErrIdentityProofRequired is a step that asks for the password, of a User who has none and
// whose session holds no fresh proof instead (ADR 0048): a Passkey check or a provider login.
var ErrIdentityProofRequired = validation.Denied("accounts: a fresh identity proof is required", "errors.identity_proof_required")

// ProveIdentity is the proof the steps that ask for the password need (ADR 0020, ADR 0048). A
// User with a password proves it through prove, the way a login does (ADR 0025): while the
// account's Login throttle delay runs it fails even for the right password, a wrong password
// counts as a failure of the account, and a right one does not reset the counter (only a
// completed login does, else a session holder could clear a run of second-step guesses). A User
// with no password proves instead what freshProof reports: a Sign-in provider login or a Passkey
// check recent enough, carried by the session; there is nothing to guess, so nothing is
// counted, and without one the step is refused with ErrIdentityProofRequired.
func ProveIdentity(ctx context.Context, u *ent.User, password string, freshProof bool, prove Prove) error {
	if u.PasswordHash != "" {
		return prove(ctx, u, password)
	}
	if freshProof {
		return nil
	}
	return ErrIdentityProofRequired
}

// ErrNewEmailSame refuses an email change to the address the User already has.
var ErrNewEmailSame = validation.RefuseField("newEmail", "accounts: the new email is the current one", "errors.new_email_must_differ")

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
	// PasswordChanged reports that the change ended every session of the User (ADR 0020): the
	// acting session continues only under a fresh one, stamped with the new epoch.
	PasswordChanged bool
}

// newPasswordForm is the new password of a profile change, checked against the password rule.
type newPasswordForm struct {
	NewPassword string `json:"newPassword" validate:"password_not_blank,password_length"`
}

// ChangeProfile changes the name and/or the password of the User u. A blank name is refused;
// a new password is checked against the password rule before the current one is proven, so a
// refused one costs no hash, and it needs the current password, proven through prove. A
// refused field is a *validation.FieldsError, and prove's refusal (a wrong current password, a
// running throttle delay) comes back as it is. Nothing is written unless every rule holds; a change that asks for nothing writes
// nothing and returns u.
func (a *Accounts) ChangeProfile(ctx context.Context, u *ent.User, c ProfileChange, prove Prove) (ProfileChanged, error) {
	var name, hash *string
	if c.Name != nil {
		trimmed := strings.TrimSpace(*c.Name)
		if trimmed == "" {
			return ProfileChanged{}, validation.Refused("name", i18n.T("errors.name_empty", nil))
		}
		name = &trimmed
	}
	if c.NewPassword != "" {
		if err := validation.Form(newPasswordForm{NewPassword: c.NewPassword}); err != nil {
			return ProfileChanged{}, err
		}
		if c.CurrentPassword == "" {
			return ProfileChanged{}, validation.Refused("currentPassword", i18n.T("errors.current_password_required", nil))
		}
		if err := prove(ctx, u, c.CurrentPassword); err != nil {
			return ProfileChanged{}, err
		}
		h, err := a.passwords.Hash(c.NewPassword)
		if err != nil {
			return ProfileChanged{}, err
		}
		hash = &h
	}
	if name == nil && hash == nil {
		return ProfileChanged{User: u}, nil
	}
	saved, err := a.updateProfile(ctx, u.ID, name, hash, false)
	if err != nil {
		return ProfileChanged{}, err
	}
	return ProfileChanged{User: saved, PasswordChanged: hash != nil}, nil
}
