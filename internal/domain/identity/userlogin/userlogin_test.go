package userlogin_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/internal/domain/identity/login"
	"github.com/getsphericon/sphericon/internal/domain/identity/passkeys"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/userlogin"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
	"github.com/getsphericon/sphericon/internal/kernel/authtoken"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

type rig struct {
	env    *testhelper.TestEnv
	module *userlogin.Module
	users  *users.Module
}

func newRig(t *testing.T) rig {
	t.Helper()
	env := testhelper.Setup(t)
	acc := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now)
	keys, err := passkeys.New(env.DB, env.Bus, acc, "https://app.example", "secret", time.Now)
	require.NoError(t, err)
	module := userlogin.New(userlogin.Config{
		Users:               acc,
		SecondFactors:       secondfactors.New(env.DB, env.Bus, env.Cipher, acc, time.Now),
		Passkeys:            keys,
		Throttle:            throttle.NewAttempts(env.DB, time.Now).For(throttle.KindLogin),
		Passwords:           testhelper.Passwords(),
		Challenges:          authtoken.New("secret", time.Now),
		RequireVerification: true,
	})
	return rig{env: env, module: module, users: acc}
}

// logins are the methods of the `user.login` entries of the User, one per Workspace entry.
func (r rig) logins(t *testing.T, userID int64) map[int64]any {
	t.Helper()
	r.env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)
	entries := r.env.DB.AuditEntry.Query().
		Where(auditentry.Action(events.ActionUserLogin), auditentry.TargetID(strconv.FormatInt(userID, 10))).
		AllX(t.Context())
	out := map[int64]any{}
	for _, e := range entries {
		out[e.WorkspaceID] = e.Diff["method"]
	}
	return out
}

// samsCode is Sam's current authenticator code.
func samsCode(t *testing.T) string {
	t.Helper()
	c, err := totp.GenerateCode(fixtures.SamSecondFactorTotpSecret, time.Now())
	require.NoError(t, err)
	return c
}

// ADR 0022: a sign-in is recorded as `user.login` in each of the User's Workspaces, naming how
// the person was proven; a refused one is not.
func TestAPasswordLoginIsRecordedInEachOfTheUsersWorkspaces(t *testing.T) {
	r := newRig(t)

	_, refused := r.module.Password(t.Context(), fixtures.MemberMaryEmail, "not her password")
	res, err := r.module.Password(t.Context(), fixtures.MemberMaryEmail, fixtures.MemberMaryPassword)

	require.ErrorIs(t, refused, login.ErrInvalidCredentials)
	require.NoError(t, err)
	assert.Equal(t, login.Authenticated, res.Outcome)
	assert.Equal(t, map[int64]any{fixtures.AcmeID: users.LoginPassword, fixtures.GlobexID: users.LoginPassword}, r.logins(t, fixtures.MemberMaryID))
}

// A User with a Second factor is not signed in by the password: the challenge is no login, and the
// second step is, by the method that began it.
func TestTheSecondStepRecordsTheLoginByTheMethodThatBeganIt(t *testing.T) {
	r := newRig(t)

	first, err := r.module.Password(t.Context(), fixtures.SecondFactorSamEmail, fixtures.SecondFactorSamPassword)
	require.NoError(t, err)
	require.Equal(t, login.SecondFactor, first.Outcome)
	assert.Empty(t, r.logins(t, fixtures.SecondFactorSamID), "the challenge alone is not a login")

	second, err := r.module.SecondFactor(t.Context(), first.Challenge, samsCode(t))

	require.NoError(t, err)
	assert.Equal(t, login.SecondAuthenticated, second.Outcome)
	assert.Equal(t, users.LoginPassword, second.Method)
	assert.Equal(t, map[int64]any{fixtures.GlobexID: users.LoginPassword, fixtures.UmbrellaID: users.LoginPassword}, r.logins(t, fixtures.SecondFactorSamID))
}

// ADR 0048: a provider sign-in that finds a Second factor due hands the second step a challenge
// that carries the provider and the time of its login, and the login is recorded by the provider.
func TestAProvidersChallengeCarriesItsMethodAndTimeToTheSecondStep(t *testing.T) {
	r := newRig(t)
	sam, err := r.users.User(t.Context(), fixtures.SecondFactorSamID)
	require.NoError(t, err)
	binding, err := secondfactors.New(r.env.DB, r.env.Bus, r.env.Cipher, r.users, time.Now).ChallengeBinding(t.Context(), sam)
	require.NoError(t, err)
	at := time.Now().Add(-time.Minute).Truncate(time.Second)
	challenge, err := r.module.Challenge(sam, binding, "google", at)
	require.NoError(t, err)

	res, err := r.module.SecondFactor(t.Context(), challenge, samsCode(t))

	require.NoError(t, err)
	assert.Equal(t, "google", res.Method)
	assert.True(t, at.Equal(res.ProviderLoginAt), "the provider login's time, %v", res.ProviderLoginAt)
	assert.Equal(t, map[int64]any{fixtures.GlobexID: "google", fixtures.UmbrellaID: "google"}, r.logins(t, fixtures.SecondFactorSamID))
}

// ADR 0030: an Unverified User is refused by the gate, and nothing is recorded.
func TestTheVerificationGateRefusesAnUnverifiedUser(t *testing.T) {
	r := newRig(t)
	u, err := r.users.UserByLoginAddress(t.Context(), fixtures.FreshUnverifiedUserEmail)
	require.NoError(t, err)

	_, err = r.module.Password(t.Context(), fixtures.FreshUnverifiedUserEmail, "whatever")

	require.ErrorIs(t, err, users.ErrEmailNotVerified)
	assert.Empty(t, r.logins(t, u.ID))
}
