package validation_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

func TestLengthCountsCharactersAndNamesTheField(t *testing.T) {
	require.NoError(t, validation.Length("message", "héllo", 1, 5))

	err := validation.Length("message", "héllo!", 1, 5)
	require.ErrorIs(t, err, validation.ErrTooLong)
	assert.Equal(t, "message must be at most 5 characters", validation.Message(err))
	refusal, ok := errors.AsType[*validation.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, "message", refusal.Field())

	require.ErrorIs(t, validation.Length("message", "", 1, 5), validation.ErrTooShort)
}

func TestRangeRefusesAValueOutsideTheBounds(t *testing.T) {
	require.NoError(t, validation.Range("perDay", 1, 1, 10))
	require.NoError(t, validation.Range("perDay", 10, 1, 10))

	err := validation.Range("perDay", 0, 1, 10)
	require.ErrorIs(t, err, validation.ErrOutOfRange)
	assert.Equal(t, "perDay must be from 1 to 10", validation.Message(err))
}

func TestItemsRefusesTooManyAndTooFew(t *testing.T) {
	require.NoError(t, validation.Items("headers", 5, 0, 5))
	require.ErrorIs(t, validation.Items("headers", 6, 0, 5), validation.ErrTooMany)
	require.ErrorIs(t, validation.Items("triggerEvents", 0, 1, 10), validation.ErrTooFew)
}

func TestPatternRefusesAValueTheContractsPatternDoesNotMatch(t *testing.T) {
	require.NoError(t, validation.Pattern("reasonCode", "review_1", `^[a-z0-9_]+$`))
	require.ErrorIs(t, validation.Pattern("reasonCode", "Review", `^[a-z0-9_]+$`), validation.ErrPatternMismatch)
}
