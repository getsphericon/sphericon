package signup_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/signup"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The verification link is single use even when two submissions race past the
// handler's own check: the write itself only lands on a still-Unverified User.
func TestVerifyEmailLandsOnlyOnAnUnverifiedUser(t *testing.T) {
	env := testhelper.Setup(t)
	acc := testhelper.Module[*users.Module](env)
	creation := signup.New(env.Bus, acc, nil, nil, testhelper.Passwords(), testhelper.Module[*suspension.DefaultSendCap](env),
		testhelper.Module[*apitokens.Module](env), testhelper.Module[*secondfactors.Module](env), signup.SignUpConfig{}, signup.SignInConfig{}, time.Now)
	u, err := acc.CreateUnverified(t.Context(), users.NewUser{Name: "Grace", Email: "grace@example.com", PasswordHash: "signed-up-hash"})
	require.NoError(t, err)

	_, err = creation.VerifyEmail(t.Context(), signup.VerifyEmailInput{UserID: u.ID, PasswordHash: new("hash-one")})
	require.NoError(t, err)
	_, err = creation.VerifyEmail(t.Context(), signup.VerifyEmailInput{UserID: u.ID, PasswordHash: new("hash-two")})
	require.ErrorIs(t, err, users.ErrAlreadyVerified)

	stored, err := acc.User(t.Context(), u.ID)
	require.NoError(t, err)
	assert.Equal(t, "hash-one", stored.PasswordHash, "the second submission overwrote nothing")
	assert.EqualValues(t, 1, stored.SessionEpoch, "verification ends earlier sessions once")
}
