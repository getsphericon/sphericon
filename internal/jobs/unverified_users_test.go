package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const unverifiedRetention = 7 * 24 * time.Hour

func pruneUnverified(t *testing.T, env *testhelper.TestEnv, retention time.Duration) {
	t.Helper()
	w := jobs.NewPruneUnverifiedUsersWorker(env.DB, retention)
	require.NoError(t, w.Work(context.Background(), &river.Job[jobs.PruneUnverifiedUsersArgs]{JobRow: &rivertype.JobRow{}}))
}

func userExists(t *testing.T, env *testhelper.TestEnv, id int64) bool {
	t.Helper()
	return env.DB.User.Query().Where(user.ID(id)).ExistX(context.Background())
}

func TestPruneUnverifiedUsersDeletesStaleOnes(t *testing.T) {
	env := testhelper.Setup(t)

	pruneUnverified(t, env, unverifiedRetention)

	assert.False(t, userExists(t, env, fixtures.StaleUnverifiedUserID))
}

func TestPruneUnverifiedUsersKeepsYoungAndVerifiedUsers(t *testing.T) {
	env := testhelper.Setup(t)
	verified := env.DB.User.Query().Where(user.EmailVerifiedAtNotNil()).CountX(context.Background())

	pruneUnverified(t, env, unverifiedRetention)

	assert.True(t, userExists(t, env, fixtures.FreshUnverifiedUserID), "younger than the window")
	assert.Equal(t, verified, env.DB.User.Query().Where(user.EmailVerifiedAtNotNil()).CountX(context.Background()),
		"a Verified User is never deleted, whatever its age")
}

func TestPruneUnverifiedUsersKeepsOneWithAMembership(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := env.DB.Scoped(fixtures.AcmeID).Membership().Create().
		SetUserID(fixtures.StaleUnverifiedMemberID).SetRole(membership.RoleMember).Save(context.Background())
	require.NoError(t, err)

	pruneUnverified(t, env, unverifiedRetention)

	assert.True(t, userExists(t, env, fixtures.StaleUnverifiedMemberID))
}

func TestPruneUnverifiedUsersDisabledByZeroRetention(t *testing.T) {
	env := testhelper.Setup(t)

	pruneUnverified(t, env, 0)

	assert.True(t, userExists(t, env, fixtures.StaleUnverifiedUserID))
}

func TestPruneUnverifiedUsersDeletesInBoundedBatches(t *testing.T) {
	env := testhelper.Setup(t)
	bulk := []string{"a@bulk.test", "b@bulk.test", "c@bulk.test"}
	for _, email := range bulk {
		_, err := env.DB.User.Create().SetName("Bulk").SetEmail(email).
			SetCreatedAt(time.Now().Add(-30 * 24 * time.Hour)).Save(t.Context())
		require.NoError(t, err)
	}

	// A batch of one forces several rounds; every stale row still goes.
	deleted, err := jobs.PruneUnverifiedUsers(t.Context(), env.DB, unverifiedRetention, 1)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, int(deleted), len(bulk), "at least the bulk Users go, one batch at a time")
	assert.Zero(t, env.DB.User.Query().Where(user.EmailIn(bulk...)).CountX(t.Context()))
}

func TestPrunedAddressKeepsItsInvitationAndTheInviteeCanAcceptIt(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus)

	pruneUnverified(t, env, unverifiedRetention)

	inv, err := env.DB.Scoped(fixtures.GlobexID).Invitation().Get(t.Context(), fixtures.InvitationStaleUnverifiedID)
	require.NoError(t, err, "an Invitation references an email, not a User")
	require.NoError(t, acc.AcceptInvitation(t.Context(), inv, "Real Owner", "a-strong-password-1", ""))
	owner, err := acc.UserByEmail(t.Context(), fixtures.StaleUnverifiedUserEmail)
	require.NoError(t, err)
	assert.NotNil(t, owner.EmailVerifiedAt)
	assert.True(t, env.DB.Scoped(fixtures.GlobexID).Membership().Query().Where(membership.UserID(owner.ID)).ExistX(t.Context()))
}

func TestAddressCanRegisterOnlyAfterItsStaleUserIsPruned(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus)
	taken, err := acc.EmailTaken(t.Context(), fixtures.StaleUnverifiedUserEmail)
	require.NoError(t, err)
	require.True(t, taken, "the address is held until pruned")

	pruneUnverified(t, env, unverifiedRetention)

	_, err = acc.Register(t.Context(), "Real Owner", fixtures.StaleUnverifiedUserEmail, "registered-hash")
	require.NoError(t, err)
}
