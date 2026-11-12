package validation_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

func TestNameTrimsTheName(t *testing.T) {
	name, err := validation.Name("  Newsletter  ")

	require.NoError(t, err)
	assert.Equal(t, "Newsletter", name)
}

func TestNameRefusesABlankNameOnTheNameField(t *testing.T) {
	for _, raw := range []string{"", "   ", "\t\n"} {
		_, err := validation.Name(raw)

		require.ErrorIs(t, err, validation.ErrNameBlank, "%q", raw)
		refusal, ok := errors.AsType[*validation.RefusalError](err)
		require.True(t, ok)
		assert.Equal(t, "name", refusal.Field())
	}
}

func TestNameCountsCharactersNotBytes(t *testing.T) {
	_, err := validation.Name(strings.Repeat("я", 100))
	require.NoError(t, err)

	_, err = validation.Name(strings.Repeat("я", 101))
	require.ErrorIs(t, err, validation.ErrNameTooLong)
	assert.Equal(t, "the name must be at most 100 characters", validation.Message(err))
}

func TestNameOptRefusesOnlyASetName(t *testing.T) {
	name, err := validation.NameOpt(nil)
	require.NoError(t, err)
	assert.Nil(t, name)

	name, err = validation.NameOpt(new(" Weekly "))
	require.NoError(t, err)
	assert.Equal(t, "Weekly", *name)

	_, err = validation.NameOpt(new(" "))
	require.ErrorIs(t, err, validation.ErrNameBlank)
}

func TestFieldsGathersTheRefusalOfEachField(t *testing.T) {
	_, nameErr := validation.Name("")
	_, emailErr := validation.Email(fieldname.Email, "not an address")

	err := validation.Fields(nameErr, nil, emailErr)

	form, ok := errors.AsType[*validation.FieldsError](err)
	require.True(t, ok)
	assert.Equal(t, []string{"name", "email"}, keys(form.Fields))
}

func TestFieldsIsNilWhenNothingIsRefused(t *testing.T) {
	assert.NoError(t, validation.Fields(nil, nil))
}

func TestFieldsPassesOnAnErrorThatIsNoRefusal(t *testing.T) {
	broken := errors.New("db down")
	_, nameErr := validation.Name("")

	assert.Equal(t, broken, validation.Fields(nameErr, broken))
}

func TestEmailKeepsTheEmailCaseAndRefusesWhatIsNoAddress(t *testing.T) {
	email, err := validation.Email(fieldname.Email, "  Grace@Example.COM ")
	require.NoError(t, err)
	assert.Equal(t, "grace@example.com", email)

	for _, raw := range []string{"grace", "Grace <grace@example.com>", "a@" + strings.Repeat("b", 320) + ".com"} {
		_, err = validation.Email(fieldname.Email, raw)
		require.ErrorIs(t, err, validation.ErrEmailInvalid, "%q", raw)
	}

	_, err = validation.Email(fieldname.NewEmail, " ")
	require.ErrorIs(t, err, validation.ErrEmailRequired)
	refusal, ok := errors.AsType[*validation.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, "newEmail", refusal.Field())
}

func TestAccountEmailIsStricterThanAStoredAddress(t *testing.T) {
	_, err := validation.Email(fieldname.Email, "fdsadsf@fdsa")
	require.NoError(t, err)

	_, err = validation.AccountEmail(fieldname.Email, "fdsadsf@fdsa")
	require.ErrorIs(t, err, validation.ErrEmailInvalid)

	email, err := validation.AccountEmail(fieldname.Email, " Grace@Example.com")
	require.NoError(t, err)
	assert.Equal(t, "grace@example.com", email)
}

func TestArgsFillsTheMessageAndKeepsTheSentinel(t *testing.T) {
	err := validation.ErrNameTooLong.Args(map[string]any{"Max": 7})

	require.ErrorIs(t, err, validation.ErrNameTooLong)
	assert.Equal(t, "the name must be at most 7 characters", validation.Message(err))
}

func keys(fields map[string][]string) []string {
	var out []string
	for _, f := range []string{"name", "email"} {
		if _, ok := fields[f]; ok {
			out = append(out, f)
		}
	}
	return out
}
