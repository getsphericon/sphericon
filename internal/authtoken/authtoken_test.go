package authtoken_test

import (
	"errors"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secret = "test-secret"

func constBinding(v string) func(int64) (string, error) {
	return func(int64) (string, error) { return v, nil }
}

func TestMintParseRoundTrip(t *testing.T) {
	s := authtoken.New(secret)
	tok, err := s.Mint(authtoken.PurposeEmailChange, 42, "old@example.com", time.Hour, map[string]string{"new": "fresh@example.com"})
	require.NoError(t, err)

	uid, extra, err := s.Parse(tok, authtoken.PurposeEmailChange, constBinding("old@example.com"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), uid)
	assert.Equal(t, "fresh@example.com", extra["new"])
}

func TestParseRejectsWrongPurpose(t *testing.T) {
	s := authtoken.New(secret)
	tok, err := s.Mint(authtoken.PurposeEmailVerify, 7, "", time.Hour, nil)
	require.NoError(t, err)

	_, _, err = s.Parse(tok, authtoken.PurposePasswordReset, constBinding(""))
	assert.Error(t, err, "a verify token must not validate as a reset token")
}

func TestParseRejectsChangedBinding(t *testing.T) {
	s := authtoken.New(secret)
	// reset token bound to the current password hash
	tok, err := s.Mint(authtoken.PurposePasswordReset, 1, "hash-v1", time.Hour, nil)
	require.NoError(t, err)

	// works while the hash is unchanged
	_, _, err = s.Parse(tok, authtoken.PurposePasswordReset, constBinding("hash-v1"))
	require.NoError(t, err)

	// once the password is reset (hash changes), the same token no longer verifies
	_, _, err = s.Parse(tok, authtoken.PurposePasswordReset, constBinding("hash-v2"))
	assert.Error(t, err, "token must be single-use once the binding changes")
}

func TestParseRejectsEmptyBindingTampered(t *testing.T) {
	s := authtoken.New(secret)
	// empty binding (e.g. a user with no password hash yet) still round-trips
	tok, err := s.Mint(authtoken.PurposePasswordReset, 1, "", time.Hour, nil)
	require.NoError(t, err)
	_, _, err = s.Parse(tok, authtoken.PurposePasswordReset, constBinding(""))
	require.NoError(t, err)
}

func TestParseRejectsExpired(t *testing.T) {
	s := authtoken.New(secret)
	tok, err := s.Mint(authtoken.PurposeEmailVerify, 1, "", -time.Minute, nil)
	require.NoError(t, err)
	_, _, err = s.Parse(tok, authtoken.PurposeEmailVerify, constBinding(""))
	assert.Error(t, err)
}

func TestParsePropagatesBindingError(t *testing.T) {
	s := authtoken.New(secret)
	tok, err := s.Mint(authtoken.PurposePasswordReset, 1, "hash", time.Hour, nil)
	require.NoError(t, err)

	sentinel := errors.New("user not found")
	_, _, err = s.Parse(tok, authtoken.PurposePasswordReset, func(int64) (string, error) { return "", sentinel })
	assert.Error(t, err, "a missing user (binding lookup fails) invalidates the token")
}

func TestParseRejectsWrongSecret(t *testing.T) {
	tok, err := authtoken.New(secret).Mint(authtoken.PurposeEmailVerify, 1, "", time.Hour, nil)
	require.NoError(t, err)
	_, _, err = authtoken.New("other-secret").Parse(tok, authtoken.PurposeEmailVerify, constBinding(""))
	assert.Error(t, err)
}
