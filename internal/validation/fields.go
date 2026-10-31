package validation

import (
	"maps"
	"slices"
	"strings"

	"github.com/go-playground/validator/v10"

	"github.com/getsphericon/sphericon/internal/i18n"
)

// FieldsError is a form a domain module refused: the failures by the json name of the field,
// each worded for a person in the instance locale. A handler returns it as it is: the server's
// error handler answers it with a 422 that carries every field (review rule 15).
type FieldsError struct {
	Fields map[string][]string
}

func (e *FieldsError) Error() string {
	return "validation: refused fields " + strings.Join(slices.Sorted(maps.Keys(e.Fields)), ", ")
}

// Detail is what a person is shown of the refusal: the one failure when there is one, and
// otherwise the failed fields, each once, named in the order the forms show them.
func (e *FieldsError) Detail() string {
	if len(e.Fields) == 1 {
		for _, messages := range e.Fields {
			if len(messages) == 1 {
				return messages[0]
			}
		}
	}
	failed := make(map[string]bool, len(e.Fields))
	for name := range e.Fields {
		failed[parentOf(name)] = true
	}
	var names []string
	for _, field := range fieldOrder {
		if failed[field] {
			names = append(names, i18n.T(fieldLabels[field], nil))
		}
	}
	return i18n.T(i18n.ErrorsCheckFields, map[string]any{"Fields": joinWords(names)})
}

// Refused is the FieldsError of one field with one message.
func Refused(field, message string) *FieldsError {
	return &FieldsError{Fields: map[string][]string{field: {message}}}
}

// Fields checks form against its validate tags and words each failure for a person: the field
// errors by json name, or nil when form is valid. An error means form cannot be validated at all.
func Fields(form any) (map[string][]string, error) {
	return Check(form, fieldMessage)
}

// Form is Fields for a domain module: a *FieldsError naming every failure, nil when form is valid.
func Form(form any) error {
	fields, err := Fields(form)
	if err != nil {
		return err
	}
	if fields != nil {
		return &FieldsError{Fields: fields}
	}
	return nil
}

// requiredMessages words an empty required field, by its json name.
var requiredMessages = map[string]i18n.ID{
	FieldName:     i18n.ErrorsNameRequired,
	FieldEmail:    i18n.ErrorsEmailRequired,
	FieldPassword: i18n.ErrorsPasswordRequired,
	FieldNewEmail: i18n.ErrorsNewEmailRequired,
}

// ruleMessages words a failed rule of this package, whatever the field.
var ruleMessages = map[string]i18n.ID{
	"account_email":      i18n.ErrorsEmailInvalid,
	"password_length":    i18n.ErrorsPasswordLength,
	"password_not_blank": i18n.ErrorsPasswordBlank,
}

// passwordLength fills the bounds into errors.password_length; the other messages ignore it.
var passwordLength = map[string]any{"Min": PasswordMinLength, "Max": PasswordMaxLength}

func fieldMessage(f validator.FieldError) string {
	if f.Tag() == "required" {
		return i18n.T(requiredMessages[f.Field()], nil)
	}
	return i18n.T(ruleMessages[f.Tag()], passwordLength)
}

// The json names of the form fields a refusal is reported against and a person is shown by
// label.
const (
	FieldName        = "name"
	FieldEmail       = "email"
	FieldNewEmail    = "newEmail"
	FieldPassword    = "password"
	FieldNewPassword = "newPassword"
	// FieldSignupItems carries the answers to the Sign-up items (ADR 0062); a refusal of one item
	// is reported under Nested(FieldSignupItems, key).
	FieldSignupItems = "signupItems"
	// FieldMaxPerSecond and FieldMaxPerDay are an Integration's Provider limit.
	FieldMaxPerSecond = "maxPerSecond"
	FieldMaxPerDay    = "maxPerDay"
)

// fieldOrder is the order fields appear in on the forms, which a detail naming them follows: the
// Sign-up items' answers come last.
var fieldOrder = []string{
	FieldName, FieldEmail, FieldNewEmail, FieldPassword, FieldNewPassword, FieldSignupItems,
	FieldMaxPerSecond, FieldMaxPerDay,
}

// fieldLabels words a field for a person, by its json name.
var fieldLabels = map[string]i18n.ID{
	FieldName:         i18n.FieldsName,
	FieldEmail:        i18n.FieldsEmail,
	FieldNewEmail:     i18n.FieldsNewEmail,
	FieldPassword:     i18n.FieldsPassword,
	FieldNewPassword:  i18n.FieldsNewPassword,
	FieldSignupItems:  i18n.FieldsSignupItems,
	FieldMaxPerSecond: i18n.FieldsMaxPerSecond,
	FieldMaxPerDay:    i18n.FieldsMaxPerDay,
}

// Nested is the name a refusal of the entry key of the request field parent is reported under
// ("signupItems.terms"): the answer lists it as a field of its own, and a person is shown the
// parent's label.
func Nested(parent, key string) string { return parent + "." + key }

// parentOf is the request field a refusal reported under name belongs to: name itself, or the
// parent of a Nested name.
func parentOf(name string) string {
	parent, _, _ := strings.Cut(name, ".")
	return parent
}

// joinWords lists words the English way, "a, b and c" (English is the only locale).
func joinWords(words []string) string {
	if len(words) < 2 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
}
