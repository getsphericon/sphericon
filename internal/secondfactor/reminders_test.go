package secondfactor_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/secondfactor"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Umbrella's requirement started 2026-03-01: Rita's grace ends 2026-03-08, Nina's (she
// joined 2026-03-05) 2026-03-12. Fixture times are local times.
var (
	ritaGraceEnds = time.Date(2026, 3, 8, 0, 0, 0, 0, time.Local)
	ninaGraceEnds = time.Date(2026, 3, 12, 0, 0, 0, 0, time.Local)
)

type reminded struct{ userIDs []int64 }

func (r *reminded) send(_ context.Context, m *ent.Membership, _ time.Time) error {
	r.userIDs = append(r.userIDs, m.UserID)
	return nil
}

func TestRemindDeadlinesClaimsEachGraceOnce(t *testing.T) {
	env := testhelper.Setup(t)
	var got reminded

	require.NoError(t, secondfactor.RemindDeadlines(t.Context(), env.DB, ritaGraceEnds.Add(-25*time.Hour), got.send))
	assert.Empty(t, got.userIDs, "more than a day before any deadline")

	for _, at := range []time.Duration{-23 * time.Hour, -22 * time.Hour, -time.Hour} {
		require.NoError(t, secondfactor.RemindDeadlines(t.Context(), env.DB, ritaGraceEnds.Add(at), got.send))
	}
	assert.Equal(t, []int64{fixtures.UmbrellaOwnerRitaID}, got.userIDs, "Rita is reminded once")

	require.NoError(t, secondfactor.RemindDeadlines(t.Context(), env.DB, ninaGraceEnds.Add(-12*time.Hour), got.send))
	assert.Equal(t, []int64{fixtures.UmbrellaOwnerRitaID, fixtures.UmbrellaMemberNinaID}, got.userIDs,
		"Nina's later Membership has its own deadline")
}

func TestRemindDeadlinesReleasesTheClaimWhenSendFails(t *testing.T) {
	env := testhelper.Setup(t)
	fail := func(context.Context, *ent.Membership, time.Time) error { return errors.New("smtp down") }
	require.Error(t, secondfactor.RemindDeadlines(t.Context(), env.DB, ritaGraceEnds.Add(-12*time.Hour), fail))

	var got reminded
	require.NoError(t, secondfactor.RemindDeadlines(t.Context(), env.DB, ritaGraceEnds.Add(-11*time.Hour), got.send))

	assert.Equal(t, []int64{fixtures.UmbrellaOwnerRitaID}, got.userIDs, "the next call retries it")
}

func TestRemindDeadlinesRemindsAgainForARestartedRequirement(t *testing.T) {
	env := testhelper.Setup(t)
	var got reminded
	require.NoError(t, secondfactor.RemindDeadlines(t.Context(), env.DB, ritaGraceEnds.Add(-12*time.Hour), got.send))

	restart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, env.DB.Workspace.UpdateOneID(fixtures.UmbrellaID).SetSecondFactorRequiredAt(restart).Exec(t.Context()))
	require.NoError(t, secondfactor.RemindDeadlines(t.Context(), env.DB, restart.Add(6*24*time.Hour+time.Hour), got.send))

	assert.Contains(t, got.userIDs[1:], int64(fixtures.UmbrellaOwnerRitaID), "Rita, reminded in the earlier grace, is reminded again")
}
