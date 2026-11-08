package secondfactors_test

import (
	"errors"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/internal/domain/identity/login"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func setup(t *testing.T) (*testhelper.TestEnv, *clock) {
	t.Helper()
	c := &clock{t: time.Now()}
	return testhelper.Setup(t, testhelper.WithClock(c.now)), c
}

func samCode(t *testing.T, c *clock) string {
	t.Helper()
	code, err := totp.GenerateCode(fixtures.SamSecondFactorTotpSecret, c.now())
	require.NoError(t, err)
	return code
}

// verify is a second step under the User's current challenge binding.
func verify(t *testing.T, env *testhelper.TestEnv, userID int64, code string) (secondfactors.Method, error) {
	t.Helper()
	return env.SecondFactor.Verify(t.Context(), pending(userID, binding(t, env, userID)), code)
}

// pending is the sign-in of the User waiting for its second step under binding.
func pending(userID int64, binding string) login.Pending[*ent.User] {
	return login.Pending[*ent.User]{Principal: &ent.User{ID: userID}, Binding: binding}
}

// binding is the User's current challenge binding ("" without a Second factor).
func binding(t *testing.T, env *testhelper.TestEnv, userID int64) string {
	t.Helper()
	u, err := env.DB.User.Get(t.Context(), userID)
	require.NoError(t, err)
	b, err := env.SecondFactor.ChallengeBinding(t.Context(), u)
	if errors.Is(err, secondfactors.ErrNotActive) {
		return ""
	}
	require.NoError(t, err)
	return b
}

func TestTheFixtureUserHasAnActiveSecondFactor(t *testing.T) {
	env, _ := setup(t)
	st, err := env.SecondFactor.Status(t.Context(), fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.Equal(t, secondfactors.Status{Enabled: true, RecoveryCodesRemaining: 1}, st)
}

func TestVerifyAcceptsEachTOTPCodeOnce(t *testing.T) {
	env, c := setup(t)

	m, err := verify(t, env, fixtures.SecondFactorSamID, samCode(t, c))
	require.NoError(t, err)
	assert.Equal(t, secondfactors.MethodTOTP, m)

	_, err = verify(t, env, fixtures.SecondFactorSamID, samCode(t, c))
	require.ErrorIs(t, err, secondfactors.ErrInvalidCode, "a replay")

	c.t = c.t.Add(30 * time.Second)
	_, err = verify(t, env, fixtures.SecondFactorSamID, samCode(t, c))
	require.NoError(t, err, "the next step")

	c.t = c.t.Add(10 * time.Minute)
	old, err := totp.GenerateCode(fixtures.SamSecondFactorTotpSecret, c.t.Add(-5*time.Minute))
	require.NoError(t, err)
	_, err = verify(t, env, fixtures.SecondFactorSamID, old)
	require.ErrorIs(t, err, secondfactors.ErrInvalidCode, "a stale code")
}

func TestVerifyAcceptsEachRecoveryCodeOnceAndRecordsItsUse(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()

	_, err := verify(t, env, fixtures.SecondFactorSamID, fixtures.SecondFactorSamSpentRecoveryCode)
	require.ErrorIs(t, err, secondfactors.ErrInvalidCode, "an already spent code")

	m, err := verify(t, env, fixtures.SecondFactorSamID, " SAM01-UNUSD ")
	require.NoError(t, err, "case and spacing do not matter")
	assert.Equal(t, secondfactors.MethodRecoveryCode, m)

	_, err = verify(t, env, fixtures.SecondFactorSamID, fixtures.SecondFactorSamRecoveryCode)
	require.ErrorIs(t, err, secondfactors.ErrInvalidCode, "a code works once")

	st, err := env.SecondFactor.Status(ctx, fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.Equal(t, 0, st.RecoveryCodesRemaining)

	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)
	n, err := env.DB.AuditEntry.Query().Where(
		auditentry.WorkspaceID(fixtures.GlobexID),
		auditentry.Action(events.ActionUserRecoveryCodeUse),
	).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

// Two second steps racing on one challenge, one with a TOTP code and one with a
// Recovery code, must not both start a session. Verify re-checks the binding under a
// lock of the User row, so the one that commits second sees the binding moved. (A
// truly concurrent test cannot run on the per-test transaction; this is the
// sequential form of the same race.)
func TestVerifyRefusesABindingAnotherSecondStepMoved(t *testing.T) {
	env, c := setup(t)
	ctx := t.Context()
	challenged := binding(t, env, fixtures.SecondFactorSamID)

	_, err := env.SecondFactor.Verify(ctx, pending(fixtures.SecondFactorSamID, challenged), fixtures.SecondFactorSamRecoveryCode)
	require.NoError(t, err)
	_, err = env.SecondFactor.Verify(ctx, pending(fixtures.SecondFactorSamID, challenged), samCode(t, c))
	require.ErrorIs(t, err, secondfactors.ErrBindingMoved)

	_, err = verify(t, env, fixtures.SecondFactorSamID, samCode(t, c))
	require.NoError(t, err, "the refused step spent nothing: the code still works")
}

func TestVerifyWithoutASecondFactor(t *testing.T) {
	env, _ := setup(t)
	_, err := verify(t, env, fixtures.OwnerJohnID, "123456")
	require.ErrorIs(t, err, secondfactors.ErrNotActive)
}
