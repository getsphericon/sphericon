package validation

import (
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/emailcase"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
)

// The refusals of the rules every owner shares. A module returns them as they are; a test matches
// them with errors.Is.
var (
	// ErrNameBlank is the name of a named row that is empty once trimmed.
	ErrNameBlank = RefuseField(fieldname.Name, "validation: name must not be empty", i18n.ErrorsNameEmpty)
	// ErrNameTooLong is the name of a named row longer than the contract's Name allows.
	ErrNameTooLong = RefuseField(fieldname.Name, "validation: name is too long", i18n.ErrorsNameTooLong)
	// ErrUserNameRequired is a User's name that is empty once trimmed.
	ErrUserNameRequired = RefuseField(fieldname.Name, "validation: name is required", i18n.ErrorsNameRequired)
	// ErrUserNameInvalid is a User's name IsDisplayName refuses: too long, empty to the eye, or
	// with a character that breaks a line or hides.
	ErrUserNameInvalid = RefuseField(fieldname.Name, "validation: invalid name", i18n.ErrorsNameInvalid)
	// ErrEmailRequired is an address that is empty once trimmed.
	ErrEmailRequired = RefuseField(fieldname.Email, "validation: email is required", i18n.ErrorsEmailRequired)
	// ErrEmailInvalid is an address that is none, or one longer than the contract allows.
	ErrEmailInvalid = RefuseField(fieldname.Email, "validation: invalid email", i18n.ErrorsEmailInvalid)
	// ErrPasswordRequired is a password left out.
	ErrPasswordRequired = RefuseField(fieldname.Password, "validation: password is required", i18n.ErrorsPasswordRequired)
	// ErrPasswordLength is a password shorter or longer than PasswordMinLength to PasswordMaxLength.
	ErrPasswordLength = RefuseField(fieldname.Password, "validation: password length", i18n.ErrorsPasswordLength)
	// ErrPasswordBlank is a password of spaces only.
	ErrPasswordBlank = RefuseField(fieldname.Password, "validation: password is blank", i18n.ErrorsPasswordBlank)
)

// Name is the rule of the name of a named row (a Tag, a Segment, a Rule, an Automation, a Template,
// a Broadcast, an Integration, a token, a Passkey, a Workspace): it is trimmed, and a blank one or
// one longer than the contract's Name is refused on the `name` field. Every owner calls it on
// create and on update.
func Name(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", ErrNameBlank
	}
	if utf8.RuneCountInString(name) > bounds.NameMaxLength {
		return "", ErrNameTooLong.Args(map[string]any{argMax: bounds.NameMaxLength})
	}
	return name, nil
}

// NameOpt is Name for an update that may leave the name out: nil stays nil.
func NameOpt(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	name, err := Name(*raw)
	if err != nil {
		return nil, err
	}
	return &name, nil
}

// NamePatch is Name for a patch field: a kept name stays kept, a set one is checked. A name is
// never cleared, so a clear is refused as blank.
func NamePatch(p patch.Field[string]) (patch.Field[string], error) {
	if p.IsKeep() {
		return p, nil
	}
	v, _ := p.Get()
	name, err := Name(v)
	if err != nil {
		return p, err
	}
	return patch.Set(name), nil
}

// UserName is the rule of a User's name, also held by a Sign-in provider's name: it is trimmed and
// refused on the `name` field when it is blank or IsDisplayName refuses it.
func UserName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", ErrUserNameRequired
	}
	if !IsDisplayName(name) {
		return "", ErrUserNameInvalid.Args(map[string]any{argNameMax: bounds.UserNameMaxLength})
	}
	return name, nil
}

// Email is the rule of a stored address, refused on the request field named field: it is put in
// its Email case, and refused when it is blank, longer than the contract's EmailAddress or no
// single bare address.
func Email(field, raw string) (string, error) {
	address := emailcase.Of(raw)
	if address == "" {
		return "", ErrEmailRequired.On(field)
	}
	if len(address) > bounds.EmailAddressMaxLength {
		return "", ErrEmailInvalid.On(field)
	}
	if parsed, err := mail.ParseAddress(address); err != nil || parsed.Address != address {
		return "", ErrEmailInvalid.On(field)
	}
	return address, nil
}

// AccountEmail is Email for the address of an account (a User's, an Invitation's, a Staff
// member's), which a person must be able to receive mail at: IsAccountEmail refuses the rest.
func AccountEmail(field, raw string) (string, error) {
	address, err := Email(field, raw)
	if err != nil {
		return "", err
	}
	if !IsAccountEmail(address) {
		return "", ErrEmailInvalid.On(field)
	}
	return address, nil
}

// Password is the rule of a new password, refused on the request field named field. It is kept as
// typed: sign-in compares the password as typed, so it is never trimmed.
func Password(field, password string) error {
	switch {
	case password == "":
		return ErrPasswordRequired.On(field)
	case !PasswordLength(password):
		return ErrPasswordLength.On(field).Args(map[string]any{argMin: PasswordMinLength, argMax: PasswordMaxLength})
	case !PasswordNotBlank(password):
		return ErrPasswordBlank.On(field)
	}
	return nil
}
