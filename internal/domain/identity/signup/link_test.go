package signup_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/login"
	"github.com/getsphericon/sphericon/internal/domain/identity/signinprovider"
	"github.com/getsphericon/sphericon/internal/domain/identity/signup"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// linkEnv is an Instance that offers Google as a Sign-in provider.
func linkEnv(t *testing.T) (*testhelper.TestEnv, *signup.Module) {
	t.Helper()
	env := testhelper.Setup(t, testhelper.WithSignin(testhelper.NewFakeSignin(t)))
	return env, testhelper.Module[*signup.Module](env)
}

// ADR 0176: a User with a password starts a link only with it, as they add a Passkey, whatever
// their session holds.
func TestALinkOfAUserWithAPasswordStartsOnlyOnThePassword(t *testing.T) {
	env, m := linkEnv(t)
	mary, err := env.DB.User.Get(t.Context(), fixtures.MemberMaryID)
	require.NoError(t, err)

	wrong, wrongErr := m.StartLink(t.Context(), mary, signup.LinkStartInput{
		Provider: "google", Proof: users.IdentityProof{Password: "not the password", ProviderLoginAt: time.Now()},
	})
	right, rightErr := m.StartLink(t.Context(), mary, signup.LinkStartInput{
		Provider: "google", Proof: users.IdentityProof{Password: fixtures.MemberMaryPassword},
	})

	require.ErrorIs(t, wrongErr, login.ErrPasswordIncorrect)
	assert.Nil(t, wrong)
	require.NoError(t, rightErr)
	assert.Equal(t, mary.ID, right.Flow.LinkUserID, "the flow names the User the callback links")
	assert.NotEmpty(t, right.AuthURL)
}

// ADR 0176: a User with no password starts a link only on a provider login or a Passkey check of
// the last few minutes.
func TestALinkOfAUserWithNoPasswordStartsOnlyOnAFreshProof(t *testing.T) {
	env, m := linkEnv(t)
	pia, err := env.DB.User.Get(t.Context(), fixtures.ProviderOnlyPiaID)
	require.NoError(t, err)

	stale, staleErr := m.StartLink(t.Context(), pia, signup.LinkStartInput{
		Provider: "google", Proof: users.IdentityProof{PasskeyCheckAt: time.Now().Add(-time.Hour)},
	})
	fresh, freshErr := m.StartLink(t.Context(), pia, signup.LinkStartInput{
		Provider: "google", Proof: users.IdentityProof{PasskeyCheckAt: time.Now()},
	})

	require.ErrorIs(t, staleErr, users.ErrIdentityProofRequired)
	assert.Nil(t, stale)
	require.NoError(t, freshErr)
	assert.Equal(t, pia.ID, fresh.Flow.LinkUserID)
}

func TestALinkOfAProviderTheInstanceDoesNotOfferIsRefused(t *testing.T) {
	env, m := linkEnv(t)
	mary, err := env.DB.User.Get(t.Context(), fixtures.MemberMaryID)
	require.NoError(t, err)

	started, err := m.StartLink(t.Context(), mary, signup.LinkStartInput{
		Provider: "myspace", Proof: users.IdentityProof{Password: fixtures.MemberMaryPassword},
	})

	require.ErrorIs(t, err, signinprovider.ErrProviderUnknown)
	assert.Nil(t, started)
}
