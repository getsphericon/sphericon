package site

import (
	"net/http"
	"strings"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/validation"
)

// checkFields validates a handler's form (a local struct tagged for internal/validation) and
// words its failures as the field errors of a 422; nil when the form is valid.
func checkFields(form any) (map[string][]string, error) {
	return validation.Fields(form)
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

// The json names of the fields a form sends and an error is reported against.
const (
	fieldName        = "name"
	fieldEmail       = "email"
	fieldNewEmail    = "newEmail"
	fieldPassword    = "password"
	fieldNewPassword = "newPassword"
)

// fieldOrder is the order fields appear in on the forms, which the detail follows.
var fieldOrder = []string{fieldName, fieldEmail, fieldNewEmail, fieldPassword, fieldNewPassword}

// fieldLabels words a field for a person, by its json name.
var fieldLabels = map[string]string{
	fieldName:        "fields.name",
	fieldEmail:       "fields.email",
	fieldNewEmail:    "fields.new_email",
	fieldPassword:    "fields.password",
	fieldNewPassword: "fields.new_password",
}

// joinWords lists words the English way, "a, b and c" (English is the only locale).
func joinWords(words []string) string {
	if len(words) < 2 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
}
