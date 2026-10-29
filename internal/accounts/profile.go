package accounts

import (
	"context"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/credentials"
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
// budget). It reports whether password is the User's; while the throttle's delay runs it
// returns that refusal as its error, even for a right password.
type Prove func(ctx context.Context, u *ent.User, password string) (bool, error)

// ErrCurrentPasswordIncorrect is a password change whose current password does not prove the
// User.
var ErrCurrentPasswordIncorrect = validation.Refuse("accounts: current password incorrect", "errors.current_password_incorrect")

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
// refused field is a *validation.FieldsError, a current password that does not prove the User
// ErrCurrentPasswordIncorrect, and prove's own error (a running throttle delay) comes back as
// it is. Nothing is written unless every rule holds; a change that asks for nothing writes
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
		proved, err := prove(ctx, u, c.CurrentPassword)
		if err != nil {
			return ProfileChanged{}, err
		}
		if !proved {
			return ProfileChanged{}, ErrCurrentPasswordIncorrect
		}
		h, err := credentials.HashPassword(c.NewPassword)
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
