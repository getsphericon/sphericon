package credentials_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/credentials"
)

func TestAPITokenRoundTripsThroughItsValueAndParses(t *testing.T) {
	prefix, err := credentials.GenerateTokenPrefix()
	require.NoError(t, err)
	secret, err := credentials.GenerateTokenSecret()
	require.NoError(t, err)
	assert.Len(t, prefix, 12)
	assert.Len(t, secret, 40)

	parsed := credentials.ParseToken(credentials.TokenValue(prefix, secret))
	require.NotNil(t, parsed)
	assert.Equal(t, prefix, parsed.Prefix)
	assert.Equal(t, secret, parsed.Secret)
}

func TestParseTokenRejectsMalformedValues(t *testing.T) {
	for _, bad := range []string{"", "omtk", "omtk_prefix", "xxxx_prefix_secret", "omtk_UPPER_secret", "omtk_prefix_sec ret", "omtk__secret"} {
		assert.Nil(t, credentials.ParseToken(bad), "%q", bad)
	}
}

func TestTokenSecretHashVerifiesOnlyTheOriginal(t *testing.T) {
	hash, err := credentials.HashTokenSecret("s3cret")
	require.NoError(t, err)
	assert.NotEqual(t, "s3cret", hash)
	assert.True(t, credentials.VerifyTokenSecret("s3cret", hash))
	assert.False(t, credentials.VerifyTokenSecret("other", hash))
	assert.False(t, credentials.VerifyTokenSecret("s3cret", "not-a-bcrypt-hash"))
}

func TestWorkspaceKeysAreTypedAndUnique(t *testing.T) {
	collect, err := credentials.GenerateCollectKey()
	require.NoError(t, err)
	ingest, err := credentials.GenerateIngestKey()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(collect, "omck_"))
	assert.True(t, strings.HasPrefix(ingest, "omik_"))
	assert.Len(t, collect, len("omck_")+40)
	assert.Len(t, ingest, len("omik_")+40)

	again, err := credentials.GenerateCollectKey()
	require.NoError(t, err)
	assert.NotEqual(t, collect, again)
}

func TestPasswordHashVerifiesOnlyTheOriginal(t *testing.T) {
	hash, err := credentials.HashPassword("correct horse")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(hash, "$argon2id$"))
	assert.True(t, credentials.VerifyPassword(hash, "correct horse"))
	assert.False(t, credentials.VerifyPassword(hash, "wrong"))
	assert.False(t, credentials.VerifyPassword("not-a-phc-hash", "correct horse"), "an undecodable hash never matches")
	assert.False(t, credentials.VerifyPassword("", ""))
}
