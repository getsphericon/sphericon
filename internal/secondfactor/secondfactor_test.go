package secondfactor_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/secondfactor"
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
	code, err := totp.GenerateCode(fixtures.SecondFactorSamTotpSecret, c.now())
	require.NoError(t, err)
	return code
}

// verify is a second step under the User's current challenge binding.
func verify(t *testing.T, env *testhelper.TestEnv, userID int64, code string) (secondfactor.Method, error) {
	t.Helper()
	return env.SecondFactor.Verify(t.Context(), userID, binding(t, env, userID), code)
}

// binding is the User's current challenge binding ("" without a Second factor).
func binding(t *testing.T, env *testhelper.TestEnv, userID int64) string {
	t.Helper()
	u, err := env.DB.User.Get(t.Context(), userID)
	require.NoError(t, err)
	b, err := env.SecondFactor.ChallengeBinding(t.Context(), u)
	if errors.Is(err, secondfactor.ErrNotActive) {
		return ""
	}
	require.NoError(t, err)
	return b
}

func TestTheFixtureUserHasAnActiveSecondFactor(t *testing.T) {
	env, _ := setup(t)
	st, err := env.SecondFactor.Status(t.Context(), fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.Equal(t, secondfactor.Status{Enabled: true, RecoveryCodesRemaining: 1}, st)
}

func TestVerifyAcceptsEachTOTPCodeOnce(t *testing.T) {
	env, c := setup(t)

	m, err := verify(t, env, fixtures.SecondFactorSamID, samCode(t, c))
	require.NoError(t, err)
	assert.Equal(t, secondfactor.MethodTOTP, m)

	_, err = verify(t, env, fixtures.SecondFactorSamID, samCode(t, c))
	require.ErrorIs(t, err, secondfactor.ErrInvalidCode, "a replay")

	c.t = c.t.Add(30 * time.Second)
	_, err = verify(t, env, fixtures.SecondFactorSamID, samCode(t, c))
	require.NoError(t, err, "the next step")

	c.t = c.t.Add(10 * time.Minute)
	old, err := totp.GenerateCode(fixtures.SecondFactorSamTotpSecret, c.t.Add(-5*time.Minute))
	require.NoError(t, err)
	_, err = verify(t, env, fixtures.SecondFactorSamID, old)
	require.ErrorIs(t, err, secondfactor.ErrInvalidCode, "a stale code")
}

func TestVerifyAcceptsEachRecoveryCodeOnceAndRecordsItsUse(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()

	_, err := verify(t, env, fixtures.SecondFactorSamID, fixtures.SecondFactorSamSpentRecoveryCode)
	require.ErrorIs(t, err, secondfactor.ErrInvalidCode, "an already spent code")

	m, err := verify(t, env, fixtures.SecondFactorSamID, " SAM01-UNUSD ")
	require.NoError(t, err, "case and spacing do not matter")
	assert.Equal(t, secondfactor.MethodRecoveryCode, m)

	_, err = verify(t, env, fixtures.SecondFactorSamID, fixtures.SecondFactorSamRecoveryCode)
	require.ErrorIs(t, err, secondfactor.ErrInvalidCode, "a code works once")

	st, err := env.SecondFactor.Status(ctx, fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.Equal(t, 0, st.RecoveryCodesRemaining)

	env.DeliverToEE(t)
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

	_, err := env.SecondFactor.Verify(ctx, fixtures.SecondFactorSamID, challenged, fixtures.SecondFactorSamRecoveryCode)
	require.NoError(t, err)
	_, err = env.SecondFactor.Verify(ctx, fixtures.SecondFactorSamID, challenged, samCode(t, c))
	require.ErrorIs(t, err, secondfactor.ErrBindingMoved)

	_, err = verify(t, env, fixtures.SecondFactorSamID, samCode(t, c))
	require.NoError(t, err, "the refused step spent nothing: the code still works")
}

func TestVerifyWithoutASecondFactor(t *testing.T) {
	env, _ := setup(t)
	_, err := verify(t, env, fixtures.OwnerJohnID, "123456")
	require.ErrorIs(t, err, secondfactor.ErrNotActive)
}

func TestResetClearsTheFactorAndEndsEverySession(t *testing.T) {
	env, c := setup(t)
	ctx := t.Context()
	session := env.SiteToken(t, fixtures.SecondFactorSamEmail, nil)

	jane := events.Actor{Kind: events.ActorUser, ID: "2", Name: "Jane"}
	require.NoError(t, env.SecondFactor.Reset(ctx, fixtures.SecondFactorSamID, jane, fixtures.GlobexID))

	st, err := env.SecondFactor.Status(ctx, fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.Equal(t, secondfactor.Status{}, st)
	_, err = verify(t, env, fixtures.SecondFactorSamID, samCode(t, c))
	require.ErrorIs(t, err, secondfactor.ErrNotActive)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/site/workspaces", nil)
	req.AddCookie(&http.Cookie{Name: "JWT", Value: session})
	env.Server.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestResetIsAuditedInTheActingWorkspaceOnly(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()
	jane := events.Actor{Kind: events.ActorUser, ID: "2", Name: "Jane"}

	require.NoError(t, env.SecondFactor.Reset(ctx, fixtures.SecondFactorSamID, jane, fixtures.GlobexID))

	env.DeliverToEE(t)
	got, err := env.DB.AuditEntry.Query().Where(auditentry.Action(events.ActionUserSecondFactorReset)).All(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, int64(fixtures.GlobexID), got[0].WorkspaceID)
	assert.Equal(t, "Jane", *got[0].ActorName)
	assert.Equal(t, "user", got[0].TargetType)
	assert.Equal(t, "Sam", *got[0].TargetName)
}

func TestResetWithoutASecondFactorChangesNothing(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()
	before, err := env.DB.User.Get(ctx, fixtures.OwnerJohnID)
	require.NoError(t, err)

	err = env.SecondFactor.Reset(ctx, fixtures.OwnerJohnID, events.Actor{Kind: events.ActorUser, ID: "2"}, fixtures.AcmeID)
	require.ErrorIs(t, err, secondfactor.ErrNotActive)

	after, err := env.DB.User.Get(ctx, fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.Equal(t, before.SessionEpoch, after.SessionEpoch, "no session ends")
}

func TestOperatorResetIsAuditedInEveryWorkspaceOfTheUser(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()

	require.NoError(t, env.SecondFactor.ResetByOperator(ctx, fixtures.SecondFactorSamID, "cli"))

	st, err := env.SecondFactor.Status(ctx, fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.False(t, st.Enabled)
	env.DeliverToEE(t)
	got, err := env.DB.AuditEntry.Query().
		Where(auditentry.Action(events.ActionUserSecondFactorReset)).
		Order(ent.Asc(auditentry.FieldWorkspaceID)).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2, "Sam belongs to Globex and Umbrella")
	assert.Equal(t, int64(fixtures.GlobexID), got[0].WorkspaceID)
	assert.Equal(t, int64(fixtures.UmbrellaID), got[1].WorkspaceID)
	for _, e := range got {
		assert.Equal(t, events.ActorOperator, e.ActorKind)
	}
}
