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
// otherwise the failed fields, named in the order the forms show them.
func (e *FieldsError) Detail() string {
	if len(e.Fields) == 1 {
		for _, messages := range e.Fields {
			if len(messages) == 1 {
				return messages[0]
			}
		}
	}
	names := make([]string, 0, len(e.Fields))
	for _, field := range fieldOrder {
		if _, failed := e.Fields[field]; failed {
			names = append(names, i18n.T(fieldLabels[field], nil))
		}
	}
	return i18n.T("errors.check_fields", map[string]any{"Fields": joinWords(names)})
}

// fieldOrder is the order fields appear in on the forms, by json name, which Detail follows.
var fieldOrder = []string{"name", "email", "newEmail", "password", "newPassword"}

// fieldLabels words a field for a person, by its json name.
var fieldLabels = map[string]string{
	"name":        "fields.name",
	"email":       "fields.email",
	"newEmail":    "fields.new_email",
	"password":    "fields.password",
	"newPassword": "fields.new_password",
}

// joinWords lists words the English way, "a, b and c" (English is the only locale).
func joinWords(words []string) string {
	if len(words) < 2 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
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
