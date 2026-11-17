package secondfactors_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/passkeys"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Umbrella's requirement started 2026-03-01: Rita's grace ends 2026-03-08, Nina's (she
// joined 2026-03-05) 2026-03-12. Fixture times are UTC.
var (
	ritaGraceEnds = time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	ninaGraceEnds = time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC)
)

// remindedUsers are the Users of the reminders the test's sweeps claimed, in claim order.
func remindedUsers(t *testing.T, env *testhelper.TestEnv) []int64 {
	t.Helper()
	reminders := testhelper.Queued[*events.SecondFactorReminderDue](t, env, events.NameMembershipSecondFactorReminderDue)
	ids := make([]int64, 0, len(reminders))
	for _, due := range reminders {
		m, err := env.DB.Membership.Get(t.Context(), due.MembershipID)
		require.NoError(t, err)
		ids = append(ids, m.UserID)
	}
	return ids
}

// remindAt runs the reminder sweep of a module whose clock reads at.
func remindAt(t *testing.T, env *testhelper.TestEnv, at time.Time) error {
	t.Helper()
	keys := testhelper.Module[*passkeys.Module](env)
	factors := secondfactors.New(env.DB, env.Bus, env.Cipher, testhelper.NewProvenUsers(env), keys, memberships.New(env.DB, env.Bus, nil, func() time.Time { return at }), func() time.Time { return at })
	return factors.RemindDeadlines(t.Context())
}

func TestRemindDeadlinesClaimsEachGraceOnce(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, remindAt(t, env, ritaGraceEnds.Add(-25*time.Hour)))
	assert.Empty(t, remindedUsers(t, env), "more than a day before any deadline")

	for _, at := range []time.Duration{-23 * time.Hour, -22 * time.Hour, -time.Hour} {
		require.NoError(t, remindAt(t, env, ritaGraceEnds.Add(at)))
	}
	assert.Equal(t, []int64{fixtures.UmbrellaOwnerRitaID}, remindedUsers(t, env), "Rita is reminded once")

	require.NoError(t, remindAt(t, env, ninaGraceEnds.Add(-12*time.Hour)))
	assert.Equal(t, []int64{fixtures.UmbrellaOwnerRitaID, fixtures.UmbrellaMemberNinaID}, remindedUsers(t, env),
		"Nina's later Membership has its own deadline")
}

// A claimed reminder carries the deadline of the grace it is for.
func TestRemindDeadlinesPublishesTheDeadline(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, remindAt(t, env, ritaGraceEnds.Add(-12*time.Hour)))

	due := testhelper.Queued[*events.SecondFactorReminderDue](t, env, events.NameMembershipSecondFactorReminderDue)
	require.Len(t, due, 1)
	reminder := due[0]
	assert.True(t, ritaGraceEnds.Equal(reminder.Deadline))
	assert.EqualValues(t, fixtures.UmbrellaID, reminder.WorkspaceID)
}

func TestRemindDeadlinesRemindsAgainForARestartedRequirement(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, remindAt(t, env, ritaGraceEnds.Add(-12*time.Hour)))

	restart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, env.DB.Workspace.UpdateOneID(fixtures.UmbrellaID).SetSecondFactorRequiredAt(restart).Exec(t.Context()))
	require.NoError(t, remindAt(t, env, restart.Add(6*24*time.Hour+time.Hour)))

	assert.Contains(t, remindedUsers(t, env)[1:], int64(fixtures.UmbrellaOwnerRitaID), "Rita, reminded in the earlier grace, is reminded again")
}
