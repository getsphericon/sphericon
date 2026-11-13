package config

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/go-playground/validator/v10"
)

// environmentValidator checks the `validate` tags of the environment and names a field by its
// variable, so a refusal reads "SPHERICON_SESSION_TTL must be positive".
var environmentValidator = func() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(variableName)
	return v
}()

// variableName is the environment variable a field is read from.
func variableName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("env"), ",")
	return name
}

// validateEnvironment refuses the values that break their field's rules, every one of them,
// in the order the fields are declared.
func validateEnvironment(e *environment) error {
	err := environmentValidator.Struct(e)
	var invalid validator.ValidationErrors
	if !errors.As(err, &invalid) {
		return err
	}
	refusals := make([]error, 0, len(invalid))
	for _, fe := range invalid {
		refusals = append(refusals, fmt.Errorf("%s %s", fe.Field(), rule(fe.Tag(), fe.Param())))
	}
	return errors.Join(refusals...)
}

// rule says what a value must be under the validator tag with its param: the refusal of a value
// that breaks it, and the rule the configuration reference lists.
func rule(tag, param string) string {
	spec := tag
	if param != "" {
		spec += "=" + param
	}
	if alternatives := strings.Split(spec, "|"); len(alternatives) > 1 {
		for i, a := range alternatives {
			alternatives[i] = alternative(a)
		}
		return "must be " + strings.Join(alternatives, " or ")
	}
	switch tag {
	case "gt":
		if param == "0" {
			return "must be positive"
		}
		return "must be above " + param
	case "gte":
		if param == "0" {
			return "must not be negative"
		}
		return "must be at least " + param
	case "lte":
		return "must be at most " + param
	case "ltfield":
		return "must be below " + fieldVariable(param)
	case "ltefield":
		return "must not be above " + fieldVariable(param)
	case "required_with":
		return "must be set together with " + fieldVariable(param)
	default:
		return fmt.Sprintf("breaks the rule %s=%s", tag, param)
	}
}

// alternative says one value an or-rule (`eq=0|gte=180`) allows: "0", "at least 180".
func alternative(a string) string {
	name, param, _ := strings.Cut(a, "=")
	switch name {
	case "eq":
		return param
	case "gte":
		return "at least " + param
	default:
		return a
	}
}

// namedByVariable names the variable in each refusal of err, env's error for the values it could
// not read as their field's type, so a refusal reads `SPHERICON_LOG_LEVEL: slog: level string
// "warning": unknown name` instead of naming the Go field. A refusal of no single field is kept
// as it is.
func namedByVariable(err error) error {
	all, ok := errors.AsType[env.AggregateError](err)
	if !ok {
		return err
	}
	refusals := make([]error, 0, len(all.Errors))
	for _, refusal := range all.Errors {
		if parse, ok := errors.AsType[env.ParseError](refusal); ok {
			refusal = fmt.Errorf("%s: %w", fieldVariable(parse.Name), parse.Err)
		}
		refusals = append(refusals, refusal)
	}
	return errors.Join(refusals...)
}

// fieldVariable is the variable of the environment field a cross-field rule names.
func fieldVariable(field string) string {
	f, ok := reflect.TypeFor[environment]().FieldByName(field)
	if !ok {
		return field
	}
	return variableName(f)
}
