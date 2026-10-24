package analytics_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/analytics"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func opensByHourAt(t *testing.T, env *testhelper.TestEnv, ws int64, w analytics.Window, now time.Time) *analytics.OpensByHour {
	t.Helper()
	m := analytics.New().WithClock(func() time.Time { return now })
	r, err := m.OpensByHour(context.Background(), env.DB.Scoped(ws), w)
	require.NoError(t, err)
	return r
}

// opens is the count of one cell, weekday 1 (Monday) to 7 (Sunday). The report holds a cell for
// every weekday and hour, Monday 00:00 first, so the cell is at a fixed position.
func opens(r *analytics.OpensByHour, weekday, hour int) int {
	return r.Cells[(weekday-1)*24+hour].Opens
}

// Fixtures (events.yml, Gotham, America/New_York): human opens at 2026-07-15 02:30Z and 02:50Z
// (Tuesday 22:30 and 22:50 EDT), a machine open at 15:00Z and a human open on 2026-01-10.
func TestOpensLandInTheHourOfTheWorkspaceZoneAndMachineOpensAreLeftOut(t *testing.T) {
	env := testhelper.Setup(t)
	r := opensByHourAt(t, env, fixtures.GothamID, analytics.Window30Days, time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC))

	assert.Equal(t, 2, opens(r, 2, 22), "Tuesday 22:00 in New York")
	assert.Equal(t, 2, r.Total, "the machine open and the open before the window are not counted")
	assert.Len(t, r.Cells, 7*24, "every weekday and hour is a cell, empty ones are zero")
	assert.Zero(t, opens(r, 2, 11))
}

func TestAZoneChangeMovesTheSameOpensToAnotherHourAndDay(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

	require.NoError(t, env.DB.Workspace.UpdateOneID(fixtures.GothamID).SetTimezone("UTC").Exec(context.Background()))
	r := opensByHourAt(t, env, fixtures.GothamID, analytics.Window30Days, now)

	assert.Equal(t, 2, opens(r, 3, 2), "Wednesday 02:00 in UTC")
	assert.Zero(t, opens(r, 2, 22))
}

func TestAWindowWithoutOpensHasNoTotal(t *testing.T) {
	env := testhelper.Setup(t)
	r := opensByHourAt(t, env, fixtures.GothamID, analytics.Window7Days, time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC))

	assert.Zero(t, r.Total)
}
