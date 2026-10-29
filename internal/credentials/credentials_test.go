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
	prefix, secret := strings.Repeat("p", 12), strings.Repeat("s", 40)
	require.NotNil(t, credentials.ParseToken("omtk_"+prefix+"_"+secret), "the well-formed base of the cases below")
	for _, bad := range []string{
		"", "omtk", "omtk_" + prefix, "xxxx_" + prefix + "_" + secret, "omtk__" + secret,
		"omtk_" + strings.ToUpper(prefix) + "_" + secret,
		"omtk_" + prefix + "_" + secret[1:] + " ",
		"omtk_" + prefix[1:] + "_" + secret, "omtk_" + prefix + "p_" + secret,
		"omtk_" + prefix + "_" + secret[1:], "omtk_" + prefix + "_" + secret + "s",
	} {
		assert.Nil(t, credentials.ParseToken(bad), "%q", bad)
	}
}

func TestGeneratedPrefixesAndSecretsDrawFromTheWholeAlphabetOnly(t *testing.T) {
	prefixChars, secretChars := map[rune]bool{}, map[rune]bool{}
	for range 300 {
		prefix, err := credentials.GenerateTokenPrefix()
		require.NoError(t, err)
		secret, err := credentials.GenerateTokenSecret()
		require.NoError(t, err)
		assert.Regexp(t, `^[a-z0-9]{12}$`, prefix)
		assert.Regexp(t, `^[A-Za-z0-9_-]{40}$`, secret)
		for _, c := range prefix {
			prefixChars[c] = true
		}
		for _, c := range secret {
			secretChars[c] = true
		}
	}
	assert.Len(t, prefixChars, 36, "every prefix character turns up")
	assert.Len(t, secretChars, 64, "every secret character turns up")
}

func TestAPITokenAndServiceCredentialDoNotParseAsEachOther(t *testing.T) {
	prefix, secret, value, err := credentials.GenerateServiceCredential()
	require.NoError(t, err)
	parsed := credentials.ParseServiceCredential(value)
	require.NotNil(t, parsed)
	assert.Equal(t, prefix, parsed.Prefix)
	assert.Equal(t, secret, parsed.Secret)

	assert.Nil(t, credentials.ParseToken(value))
	assert.Nil(t, credentials.ParseServiceCredential(credentials.TokenValue(prefix, secret)))
}

func TestSecretHashVerifiesOnlyTheOriginal(t *testing.T) {
	hash := credentials.HashSecret("s3cret")
	assert.NotEqual(t, "s3cret", hash)
	assert.True(t, credentials.VerifySecret("s3cret", hash))
	assert.False(t, credentials.VerifySecret("other", hash))
	assert.False(t, credentials.VerifySecret("s3cret", "not-a-sha256-hex"))
	assert.False(t, credentials.VerifySecret("s3cret", ""))
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
	hash, err := credentials.NewHasher().Hash("correct horse")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(hash, "$argon2id$"))
	assert.True(t, credentials.VerifyPassword(hash, "correct horse"))
	assert.False(t, credentials.VerifyPassword(hash, "wrong"))
	assert.False(t, credentials.VerifyPassword("not-a-phc-hash", "correct horse"), "an undecodable hash never matches")
	assert.False(t, credentials.VerifyPassword("", ""))
}
