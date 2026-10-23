package validation_test

import (
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/validation"
)

type form struct {
	Name    string `json:"name,omitempty" validate:"required"`
	Untyped string `validate:"required"`
}

func TestStructNamesAFieldByItsJSONTag(t *testing.T) {
	var failures validator.ValidationErrors
	require.ErrorAs(t, validation.Struct(form{}), &failures)
	fields := lo.Map(failures, func(f validator.FieldError, _ int) string { return f.Field() })
	assert.Equal(t, []string{"name", "Untyped"}, fields)
}

func TestStructAcceptsAValidValue(t *testing.T) {
	assert.NoError(t, validation.Struct(form{Name: "a", Untyped: "b"}))
}

func tagOf(f validator.FieldError) string { return f.Tag() }

func TestCheckWordsEachFailedFieldByItsJSONName(t *testing.T) {
	fields, err := validation.Check(form{Untyped: "b"}, tagOf)
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{"name": {"required"}}, fields)
}

func TestCheckReturnsNoFieldsForAValidValue(t *testing.T) {
	fields, err := validation.Check(form{Name: "a", Untyped: "b"}, tagOf)
	require.NoError(t, err)
	assert.Nil(t, fields)
}

func TestCheckRefusesAValueItCannotValidate(t *testing.T) {
	_, err := validation.Check("not a struct", tagOf)
	var invalid *validator.InvalidValidationError
	assert.ErrorAs(t, err, &invalid)
}
