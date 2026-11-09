package passkeys_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/passkey"
	"github.com/getsphericon/sphericon/internal/domain/identity/login"
	"github.com/getsphericon/sphericon/internal/domain/identity/passkeys"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func setup(t *testing.T) (*testhelper.TestEnv, *passkeys.Module) {
	t.Helper()
	env := testhelper.Setup(t)
	// A User proves their identity the way the server's Users do: through the login module's
	// prove path, over the Login throttle.
	prover := login.New(login.Config[*ent.User]{
		Throttle: throttle.NewAttempts(env.DB, time.Now).For(throttle.KindLogin), Passwords: testhelper.Passwords(),
	})
	identity := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now, users.WithProver(prover))
	// The ceremony secret only seals the cookies this module opens again.
	m, err := passkeys.New(env.DB, env.Bus, identity, env.AppURL, "passkeys-test-ceremony-secret", time.Now)
	require.NoError(t, err)
	return env, m
}

func user(t *testing.T, env *testhelper.TestEnv, id int64) *ent.User {
	t.Helper()
	u, err := env.DB.User.Get(t.Context(), id)
	require.NoError(t, err)
	return u
}

func passkeysOf(t *testing.T, env *testhelper.TestEnv, userID int64) int {
	t.Helper()
	n, err := env.DB.Passkey.Query().Where(passkey.UserID(userID)).Count(t.Context())
	require.NoError(t, err)
	return n
}

// A ceremony finished twice spends its challenge once: passkey_challenges_challenge_key refuses
// the second with ErrChallengeSpent, and only one Passkey is stored.
func TestARegistrationFinishedTwiceIsRefusedAsSpent(t *testing.T) {
	env, m := setup(t)
	pat := user(t, env, fixtures.PasskeyPatID)
	options, ceremony, err := m.BeginRegistration(t.Context(), pat)
	require.NoError(t, err)
	response := env.NewAuthenticator(t).Attest(options)
	_, _, err = m.FinishRegistration(t.Context(), pat, users.IdentityProof{Password: fixtures.PasskeyPatPassword}, passkeys.Registration{Ceremony: ceremony, Response: response, Name: "Pat's phone"})
	require.NoError(t, err)
	held := passkeysOf(t, env, pat.ID)

	_, _, err = m.FinishRegistration(t.Context(), pat, users.IdentityProof{Password: fixtures.PasskeyPatPassword}, passkeys.Registration{Ceremony: ceremony, Response: response, Name: "Pat's phone"})

	require.ErrorIs(t, err, passkeys.ErrChallengeSpent)
	require.ErrorIs(t, err, passkeys.ErrRegistrationFailed)
	after := passkeysOf(t, env, pat.ID)
	assert.Equal(t, held, after)
}

// A credential stored for one User is refused for another by passkeys_credential_id_key: the
// registration fails with ErrCredentialRegistered and stores nothing.
func TestACredentialRegisteredBeforeIsRefused(t *testing.T) {
	env, m := setup(t)
	john := user(t, env, fixtures.OwnerJohnID)
	options, ceremony, err := m.BeginRegistration(t.Context(), john)
	require.NoError(t, err)
	// Pat's laptop answers John's ceremony with the credential it registered for Pat.
	response := env.FixturePasskey(t).AttestHeld(options)
	held := passkeysOf(t, env, john.ID)

	_, _, err = m.FinishRegistration(t.Context(), john, users.IdentityProof{Password: fixtures.OwnerJohnPassword}, passkeys.Registration{Ceremony: ceremony, Response: response, Name: "Borrowed laptop"})

	require.ErrorIs(t, err, passkeys.ErrCredentialRegistered)
	require.ErrorIs(t, err, passkeys.ErrRegistrationFailed)
	after := passkeysOf(t, env, john.ID)
	assert.Equal(t, held, after)
}

// A Passkey is removed only once its User proves their identity, and the removal says whether the
// User holds a Passkey still, so the acting session drops its Passkey proof with the last one.
func TestRemovingAPasskeyTakesTheIdentityProofAndTellsWhetherNoneIsLeft(t *testing.T) {
	env, m := setup(t)
	pat := user(t, env, fixtures.PasskeyPatID)

	_, refused := m.Remove(t.Context(), pat, users.IdentityProof{Password: "wrong"}, fixtures.PasskeyPatLaptopID)
	kept := passkeysOf(t, env, pat.ID)
	removal, err := m.Remove(t.Context(), pat, users.IdentityProof{Password: fixtures.PasskeyPatPassword}, fixtures.PasskeyPatLaptopID)

	require.ErrorIs(t, refused, login.ErrPasswordIncorrect)
	assert.Equal(t, 1, kept)
	require.NoError(t, err)
	assert.True(t, removal.NoneLeft)
	assert.Equal(t, int64(fixtures.PasskeyPatLaptopID), removal.Passkey.ID)
	assert.Equal(t, pat.SessionEpoch+1, removal.User.SessionEpoch)
	assert.Zero(t, passkeysOf(t, env, pat.ID))
}

func TestANameLeftBlankRefusesTheRegistrationBeforeAnyProof(t *testing.T) {
	env, m := setup(t)
	pat := user(t, env, fixtures.PasskeyPatID)

	_, _, err := m.FinishRegistration(t.Context(), pat, users.IdentityProof{Password: "wrong"}, passkeys.Registration{Name: "  "})

	require.ErrorIs(t, err, passkeys.ErrPasskeyNameEmpty)
}
