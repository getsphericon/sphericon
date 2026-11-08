package secondfactors_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	reminders := env.QueuedEvents(t, events.NameMembershipSecondFactorReminderDue)
	ids := make([]int64, 0, len(reminders))
	for _, ev := range reminders {
		due, ok := ev.(*events.SecondFactorReminderDue)
		require.True(t, ok)
		m, err := env.DB.Membership.Get(t.Context(), due.MembershipID)
		require.NoError(t, err)
		ids = append(ids, m.UserID)
	}
	return ids
}

func TestRemindDeadlinesClaimsEachGraceOnce(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, newModule(env).RemindDeadlines(t.Context(), ritaGraceEnds.Add(-25*time.Hour)))
	assert.Empty(t, remindedUsers(t, env), "more than a day before any deadline")

	for _, at := range []time.Duration{-23 * time.Hour, -22 * time.Hour, -time.Hour} {
		require.NoError(t, newModule(env).RemindDeadlines(t.Context(), ritaGraceEnds.Add(at)))
	}
	assert.Equal(t, []int64{fixtures.UmbrellaOwnerRitaID}, remindedUsers(t, env), "Rita is reminded once")

	require.NoError(t, newModule(env).RemindDeadlines(t.Context(), ninaGraceEnds.Add(-12*time.Hour)))
	assert.Equal(t, []int64{fixtures.UmbrellaOwnerRitaID, fixtures.UmbrellaMemberNinaID}, remindedUsers(t, env),
		"Nina's later Membership has its own deadline")
}

// A claimed reminder carries the deadline of the grace it is for.
func TestRemindDeadlinesPublishesTheDeadline(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, newModule(env).RemindDeadlines(t.Context(), ritaGraceEnds.Add(-12*time.Hour)))

	due := env.QueuedEvents(t, events.NameMembershipSecondFactorReminderDue)
	require.Len(t, due, 1)
	reminder, ok := due[0].(*events.SecondFactorReminderDue)
	require.True(t, ok)
	assert.True(t, ritaGraceEnds.Equal(reminder.Deadline))
	assert.EqualValues(t, fixtures.UmbrellaID, reminder.WorkspaceID)
}

func TestRemindDeadlinesRemindsAgainForARestartedRequirement(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, newModule(env).RemindDeadlines(t.Context(), ritaGraceEnds.Add(-12*time.Hour)))

	restart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, env.DB.Workspace.UpdateOneID(fixtures.UmbrellaID).SetSecondFactorRequiredAt(restart).Exec(t.Context()))
	require.NoError(t, newModule(env).RemindDeadlines(t.Context(), restart.Add(6*24*time.Hour+time.Hour)))

	assert.Contains(t, remindedUsers(t, env)[1:], int64(fixtures.UmbrellaOwnerRitaID), "Rita, reminded in the earlier grace, is reminded again")
}
