package secondfactorreset_test

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
	"github.com/getsphericon/sphericon/internal/domain/identity/login"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/secondfactorreset"
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
	u, err := env.DB.User.Get(t.Context(), userID)
	require.NoError(t, err)
	b, err := env.SecondFactor.ChallengeBinding(t.Context(), u)
	if !errors.Is(err, secondfactors.ErrNotActive) {
		require.NoError(t, err)
	}
	return env.SecondFactor.Verify(t.Context(), login.Pending[*ent.User]{Principal: &ent.User{ID: userID}, Binding: b}, code)
}

func TestResetClearsTheFactorAndEndsEverySession(t *testing.T) {
	env, c := setup(t)
	ctx := t.Context()
	session := env.SiteToken(t, fixtures.SecondFactorSamEmail, nil)

	jane := events.Actor{Kind: events.ActorUser, ID: "2", Name: "Jane"}
	require.NoError(t, env.SecondFactorReset.Reset(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.SecondFactorSamID, jane))

	st, err := env.SecondFactor.Status(ctx, fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.Equal(t, secondfactors.Status{}, st)
	_, err = verify(t, env, fixtures.SecondFactorSamID, samCode(t, c))
	require.ErrorIs(t, err, secondfactors.ErrNotActive)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/site/workspaces", nil)
	req.AddCookie(testhelper.RequestCookie("JWT", session))
	env.Server.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestResetIsAuditedInTheActingWorkspaceOnly(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()
	jane := events.Actor{Kind: events.ActorUser, ID: "2", Name: "Jane"}

	require.NoError(t, env.SecondFactorReset.Reset(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.SecondFactorSamID, jane))

	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)
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

	err = env.SecondFactorReset.Reset(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.OwnerJohnID, events.Actor{Kind: events.ActorUser, ID: "2"})
	require.ErrorIs(t, err, secondfactorreset.ErrNothingToReset)

	after, err := env.DB.User.Get(ctx, fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.Equal(t, before.SessionEpoch, after.SessionEpoch, "no session ends")
}

func TestStaffResetIsAuditedInEveryWorkspaceOfTheUser(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()

	before := env.DB.User.GetX(ctx, fixtures.SecondFactorSamID)

	require.NoError(t, env.SecondFactorReset.ResetByStaff(ctx, "Sam@Globex.test", "cli"), "the email is matched whatever its case")

	st, err := env.SecondFactor.Status(ctx, fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.False(t, st.Enabled)
	factor := env.DB.SecondFactor.GetX(ctx, fixtures.SamSecondFactorID)
	assert.Nil(t, factor.ConfirmedAt)
	assert.Empty(t, factor.SecretEncrypted)
	assert.Greater(t, env.DB.User.GetX(ctx, fixtures.SecondFactorSamID).SessionEpoch, before.SessionEpoch, "every session ends")
	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)
	got, err := env.DB.AuditEntry.Query().
		Where(auditentry.Action(events.ActionUserSecondFactorReset)).
		Order(ent.Asc(auditentry.FieldWorkspaceID)).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2, "Sam belongs to Globex and Umbrella")
	assert.Equal(t, int64(fixtures.GlobexID), got[0].WorkspaceID)
	assert.Equal(t, int64(fixtures.UmbrellaID), got[1].WorkspaceID)
	for _, e := range got {
		assert.Equal(t, events.ActorStaff, e.ActorKind)
	}
}

func TestStaffResetOfAUserWithNothingToClearChangesNothing(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()
	require.NoError(t, env.SecondFactorReset.ResetByStaff(ctx, fixtures.SecondFactorSamEmail, "cli"))

	err := env.SecondFactorReset.ResetByStaff(ctx, fixtures.SecondFactorSamEmail, "cli")

	require.ErrorIs(t, err, secondfactorreset.ErrNothingToReset)
}

func TestStaffResetOfAnUnknownEmailNamesIt(t *testing.T) {
	env, _ := setup(t)

	err := env.SecondFactorReset.ResetByStaff(t.Context(), "nobody@app-2fa-reset.test", "cli")

	require.ErrorContains(t, err, `user "nobody@app-2fa-reset.test"`)
	require.True(t, ent.IsNotFound(err))
}
