package site

import (
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/validation"
)

// checkFields validates a handler's form (a local struct tagged for internal/validation) and
// words its failures as the field errors of a 422; nil when the form is valid.
func checkFields(form any) (map[string][]string, error) {
	return validation.Check(form, fieldMessage)
}

// fieldsProblem is the 422 for a form's field errors. Its detail, which is what the SPA shows,
// is the one failure when there is one, and otherwise names the failed fields.
func fieldsProblem(fields map[string][]string) siteapi.ProblemDetails {
	return problemWithErrors(http.StatusUnprocessableEntity, fieldsDetail(fields), fields)
}

func fieldsDetail(fields map[string][]string) string {
	if len(fields) == 1 {
		for _, messages := range fields {
			if len(messages) == 1 {
				return messages[0]
			}
		}
	}
	names := make([]string, 0, len(fields))
	for _, field := range fieldOrder {
		if _, failed := fields[field]; failed {
			names = append(names, i18n.T(fieldLabels[field], nil))
		}
	}
	return i18n.T("errors.check_fields", map[string]any{"Fields": joinWords(names)})
}

// fieldOrder is the order fields appear in on the forms, which the detail follows.
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

// requiredMessages words an empty required field, by its json name.
var requiredMessages = map[string]string{
	"name":     "errors.name_required",
	"email":    "errors.email_required",
	"password": "errors.password_required",
	"newEmail": "errors.new_email_required",
}

// ruleMessages words a failed rule of internal/validation, whatever the field.
var ruleMessages = map[string]string{
	"account_email":      "errors.email_invalid",
	"password_length":    "errors.password_length",
	"password_not_blank": "errors.password_blank",
}

// passwordLength fills the bounds into errors.password_length; the other messages ignore it.
var passwordLength = map[string]any{"Min": validation.PasswordMinLength, "Max": validation.PasswordMaxLength}

func fieldMessage(f validator.FieldError) string {
	if f.Tag() == "required" {
		return i18n.T(requiredMessages[f.Field()], nil)
	}
	return i18n.T(ruleMessages[f.Tag()], passwordLength)
}
