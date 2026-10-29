package validation

import (
	"maps"
	"slices"
	"strings"

	"github.com/go-playground/validator/v10"

	"github.com/getsphericon/sphericon/internal/i18n"
)

// FieldsError is a form a domain module refused: the failures by the json name of the field,
// each worded for a person in the instance locale. A handler answers it with a 422 that carries
// every field.
type FieldsError struct {
	Fields map[string][]string
}

func (e *FieldsError) Error() string {
	return "validation: refused fields " + strings.Join(slices.Sorted(maps.Keys(e.Fields)), ", ")
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
var requiredMessages = map[string]string{
	"name":     "errors.name_required",
	"email":    "errors.email_required",
	"password": "errors.password_required",
	"newEmail": "errors.new_email_required",
}

// ruleMessages words a failed rule of this package, whatever the field.
var ruleMessages = map[string]string{
	"account_email":      "errors.email_invalid",
	"password_length":    "errors.password_length",
	"password_not_blank": "errors.password_blank",
}

// passwordLength fills the bounds into errors.password_length; the other messages ignore it.
var passwordLength = map[string]any{"Min": PasswordMinLength, "Max": PasswordMaxLength}

func fieldMessage(f validator.FieldError) string {
	if f.Tag() == "required" {
		return i18n.T(requiredMessages[f.Field()], nil)
	}
	return i18n.T(ruleMessages[f.Tag()], passwordLength)
}
