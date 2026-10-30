package erasure_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/erasure"
)

func TestAnErasureByAnIdentifierNamesExactlyOne(t *testing.T) {
	email, visitor, blank := "a@example.com", "vis-1", ""

	byEmail, err := erasure.ByEmailOrVisitorID(&email, nil)
	require.NoError(t, err)
	assert.Equal(t, erasure.ByEmail(email), byEmail)
	byVisitor, err := erasure.ByEmailOrVisitorID(nil, &visitor)
	require.NoError(t, err)
	assert.Equal(t, erasure.ByVisitorID(visitor), byVisitor)

	_, neither := erasure.ByEmailOrVisitorID(nil, nil)
	_, both := erasure.ByEmailOrVisitorID(&email, &visitor)
	_, empty := erasure.ByEmailOrVisitorID(nil, &blank)
	require.ErrorIs(t, neither, erasure.ErrIdentifier)
	require.ErrorIs(t, both, erasure.ErrIdentifier)
	require.ErrorIs(t, empty, erasure.ErrIdentifier, "an empty visitor id names nothing")
}
