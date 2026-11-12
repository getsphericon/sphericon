package validation

import (
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
)

// FieldsError is a form a domain module refused: the failures by the json name of the field,
// each worded for a person in the instance locale; Fields makes it. A handler returns it as it is:
// the server's error handler answers it with a 422 that carries every field (review rule 15).
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

// Fields gathers the refusals of a form's fields into one *FieldsError naming every refused field,
// or nil when none is refused. An argument is nil (the field passed), a refusal of one field
// (KindRefused, with a Field) or a *FieldsError; any other error is returned as it is, so a check
// that could not run is never mistaken for a refused form.
func Fields(refusals ...error) error {
	fields := map[string][]string{}
	for _, err := range refusals {
		if err == nil {
			continue
		}
		if form, ok := errors.AsType[*FieldsError](err); ok {
			for name, messages := range form.Fields {
				fields[name] = append(fields[name], messages...)
			}
			continue
		}
		refused, ok := errors.AsType[*RefusalError](err)
		if !ok || refused.Kind() != KindRefused || refused.Field() == "" {
			return err
		}
		fields[refused.Field()] = append(fields[refused.Field()], Message(err))
	}
	if len(fields) == 0 {
		return nil
	}
	return &FieldsError{Fields: fields}
}

// fieldOrder is the order the fields a person is shown by label appear in on the forms, which a
// detail naming them follows: the Sign-up items' answers (fieldname.SignupItems, a refusal of one
// item reported under Nested(fieldname.SignupItems, key)) come last.
var fieldOrder = []string{
	fieldname.Name, fieldname.Email, fieldname.NewEmail, fieldname.Password, fieldname.NewPassword, fieldname.SignupItems,
	fieldname.MaxPerSecond, fieldname.MaxPerDay,
}

// fieldLabels words a field for a person, by its json name.
var fieldLabels = map[string]i18n.ID{
	fieldname.Name:         i18n.FieldsName,
	fieldname.Email:        i18n.FieldsEmail,
	fieldname.NewEmail:     i18n.FieldsNewEmail,
	fieldname.Password:     i18n.FieldsPassword,
	fieldname.NewPassword:  i18n.FieldsNewPassword,
	fieldname.SignupItems:  i18n.FieldsSignupItems,
	fieldname.MaxPerSecond: i18n.FieldsMaxPerSecond,
	fieldname.MaxPerDay:    i18n.FieldsMaxPerDay,
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
