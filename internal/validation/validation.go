// Package validation checks request fields beyond what the API contract states: ogen has
// already decoded the body and applied the contract's constraints, and a handler copies the
// fields it checks further into a local struct tagged for go-playground/validator. Errors name
// a field by its json tag, the name the contract and the client use. A domain module that refuses
// an input on a rule of its own returns an Error, which a handler answers with a 422 naming the field.
package validation

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// validate is safe for concurrent use once its rules are registered, which happens here only.
var validate = newValidator()

func newValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(jsonName)
	for tag, ok := range map[string]func(string) bool{
		"account_email":      AccountEmail,
		"display_name":       DisplayName,
		"password_length":    PasswordLength,
		"password_not_blank": PasswordNotBlank,
	} {
		// A rule that fails to register would silently not check: refuse to start instead.
		if err := v.RegisterValidation(tag, func(fl validator.FieldLevel) bool { return ok(fl.Field().String()) }); err != nil {
			panic(fmt.Sprintf("validation: register %s: %v", tag, err))
		}
	}
	return v
}

// jsonName is a field's name in the request body: its json tag, the Go name when it has none,
// and "" (the field is skipped) for json:"-".
func jsonName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	switch name {
	case "-":
		return ""
	case "":
		return f.Name
	}
	return name
}

// Struct checks s against its validate tags. A failure is a validator.ValidationErrors whose
// Field() is the json name.
func Struct(s any) error {
	return validate.Struct(s)
}

// Check checks s and returns its failures as field errors, json name to messages, or nil when
// s is valid. message words one failure. An error means s cannot be validated at all (not a
// struct, a bad tag): a bug in the caller, never the request's fault.
func Check(s any, message func(validator.FieldError) string) (map[string][]string, error) {
	err := validate.Struct(s)
	if err == nil {
		return nil, nil
	}
	failures, ok := errors.AsType[validator.ValidationErrors](err)
	if !ok {
		return nil, err
	}
	fields := make(map[string][]string, len(failures))
	for _, f := range failures {
		fields[f.Field()] = append(fields[f.Field()], message(f))
	}
	return fields, nil
}
