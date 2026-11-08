package passkeys_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/passkey"
	"github.com/getsphericon/sphericon/internal/domain/identity/passkeys"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func setup(t *testing.T) (*testhelper.TestEnv, *passkeys.Module) {
	t.Helper()
	env := testhelper.Setup(t)
	// The ceremony secret only seals the cookies this module opens again.
	m, err := passkeys.New(env.DB, env.Bus, env.AppURL, "passkeys-test-ceremony-secret", time.Now)
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
	_, _, err = m.FinishRegistration(t.Context(), pat.ID, ceremony, response, "Pat's phone")
	require.NoError(t, err)
	held := passkeysOf(t, env, pat.ID)

	_, _, err = m.FinishRegistration(t.Context(), pat.ID, ceremony, response, "Pat's phone")

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

	_, _, err = m.FinishRegistration(t.Context(), john.ID, ceremony, response, "Borrowed laptop")

	require.ErrorIs(t, err, passkeys.ErrCredentialRegistered)
	require.ErrorIs(t, err, passkeys.ErrRegistrationFailed)
	after := passkeysOf(t, env, john.ID)
	assert.Equal(t, held, after)
}
