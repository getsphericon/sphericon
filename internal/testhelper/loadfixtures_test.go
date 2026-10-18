package testhelper

import (
	"database/sql"
	"testing"
	"testing/synctest"
	"time"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/stretchr/testify/require"
)

// A test package whose first test runs inside a testing/synctest bubble loads the fixtures
// there, and the bubble's clock starts in the year 2000: dated by it, every fixture "now"
// would be decades old and any test that depends on a date fails, in an order-dependent
// way (the shuffle seed decides which test is first).
func TestFixturesLoadedInsideASynctestBubbleAreDatedByTheRealClock(t *testing.T) {
	cfg, err := config.Load("test")
	require.NoError(t, err)
	url := EmptyDBURL(t)
	cfg.DatabaseURL = url
	before := time.Now()

	synctest.Test(t, func(t *testing.T) {
		sqlDB, err := sql.Open("pgx", url)
		require.NoError(t, err)
		t.Cleanup(func() { _ = sqlDB.Close() })
		require.NoError(t, migrateSchema(t.Context(), sqlDB))
		require.NoError(t, loadFixtures(t.Context(), sqlDB, cfg, projectRoot()))

		row, err := db.NewEntClient(sqlDB).AuthAttempt.Get(t.Context(), fixtures.FreshLoginAttemptID)
		require.NoError(t, err)
		require.WithinDuration(t, before, row.LastAttemptAt, time.Minute, "the fixture is dated 'now', by the real clock")
	})
}
