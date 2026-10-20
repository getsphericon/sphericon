package authtoken_test

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/authtoken"
)

func forge(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("whatever"))
	require.NoError(t, err)
	return tok
}

// The binding is looked up from the claimed uid, so a token without a usable uid must
// be refused before any lookup happens.
func TestParseRejectsTokensWithoutAUsableUserID(t *testing.T) {
	s := authtoken.New(secret)
	lookups := 0
	binding := func(int64) (string, error) { lookups++; return "b", nil }
	exp := int64(4102444800)

	cases := map[string]jwt.MapClaims{
		"no uid":      {"prp": string(authtoken.PurposeEmailVerify), "exp": exp},
		"numeric uid": {"uid": 42, "prp": string(authtoken.PurposeEmailVerify), "exp": exp},
		"non-int uid": {"uid": "abc", "prp": string(authtoken.PurposeEmailVerify), "exp": exp},
		"no purpose":  {"uid": "42", "exp": exp},
	}
	for name, claims := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := s.Parse(forge(t, claims), authtoken.PurposeEmailVerify, binding)
			require.Error(t, err)
		})
	}
	assert.Zero(t, lookups)
}

func TestParseRejectsGarbageAndUnsignedTokens(t *testing.T) {
	s := authtoken.New(secret)
	_, _, err := s.Parse("not.a.jwt", authtoken.PurposeEmailVerify, constBinding("b"))
	require.Error(t, err)

	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"uid": "1", "prp": "email_verify", "exp": int64(4102444800)}).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)
	_, _, err = s.Parse(unsigned, authtoken.PurposeEmailVerify, constBinding("b"))
	assert.Error(t, err, "alg=none must never verify")
}
