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

func TestLengthCountsCharactersAndNamesTheField(t *testing.T) {
	require.NoError(t, validation.Length(fieldname.Message, "héllo", 1, 5))

	err := validation.Length(fieldname.Message, "héllo!", 1, 5)
	require.ErrorIs(t, err, validation.ErrTooLong)
	assert.Equal(t, "message must be at most 5 characters", validation.Message(err))
	refusal, ok := errors.AsType[*validation.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, "message", refusal.Field())

	require.ErrorIs(t, validation.Length(fieldname.Message, "", 1, 5), validation.ErrTooShort)
}

func TestMinLengthRefusesOnlyAValueTooShort(t *testing.T) {
	require.NoError(t, validation.MinLength("host", strings.Repeat("a", 1000), 1))

	err := validation.MinLength("host", "", 1)
	require.ErrorIs(t, err, validation.ErrTooShort)
	refusal, ok := errors.AsType[*validation.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, "host", refusal.Field())
}

func TestRangeRefusesAValueOutsideTheBounds(t *testing.T) {
	require.NoError(t, validation.Range(fieldname.PerDay, 1, 1, 10))
	require.NoError(t, validation.Range(fieldname.PerDay, 10, 1, 10))

	err := validation.Range(fieldname.PerDay, 0, 1, 10)
	require.ErrorIs(t, err, validation.ErrOutOfRange)
	assert.Equal(t, "perDay must be from 1 to 10", validation.Message(err))
}

func TestItemsRefusesTooManyAndTooFew(t *testing.T) {
	require.NoError(t, validation.Items(fieldname.Headers, 5, 0, 5))
	require.ErrorIs(t, validation.Items(fieldname.Headers, 6, 0, 5), validation.ErrTooMany)
	require.ErrorIs(t, validation.Items(fieldname.TriggerEvents, 0, 1, 10), validation.ErrTooFew)
}

func TestPatternRefusesAValueTheContractsPatternDoesNotMatch(t *testing.T) {
	require.NoError(t, validation.Pattern(fieldname.ReasonCode, "review_1", `^[a-z0-9_]+$`))
	require.ErrorIs(t, validation.Pattern(fieldname.ReasonCode, "Review", `^[a-z0-9_]+$`), validation.ErrPatternMismatch)
}
