package contacterasure_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/usecase/contacterasure"
)

func TestAnErasureByAnIdentifierNamesExactlyOne(t *testing.T) {
	email, visitor, blank := "a@example.com", "vis-1", ""

	byEmail, err := contacterasure.ByEmailOrVisitorID(&email, nil)
	require.NoError(t, err)
	assert.Equal(t, contacterasure.ByEmail(email), byEmail)
	byVisitor, err := contacterasure.ByEmailOrVisitorID(nil, &visitor)
	require.NoError(t, err)
	assert.Equal(t, contacterasure.ByVisitorID(visitor), byVisitor)

	_, neither := contacterasure.ByEmailOrVisitorID(nil, nil)
	_, both := contacterasure.ByEmailOrVisitorID(&email, &visitor)
	_, empty := contacterasure.ByEmailOrVisitorID(nil, &blank)
	require.ErrorIs(t, neither, contacterasure.ErrIdentifier)
	require.ErrorIs(t, both, contacterasure.ErrIdentifier)
	require.ErrorIs(t, empty, contacterasure.ErrIdentifier, "an empty visitor id names nothing")
}
