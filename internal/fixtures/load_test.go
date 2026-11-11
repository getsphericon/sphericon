package fixtures_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/usertombstone"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A test package whose first test runs inside a testing/synctest bubble loads the fixtures
// there, and the bubble's clock starts in the year 2000: dated by it, every fixture "now"
// would be decades old and any test that depends on a date fails, in an order-dependent
// way (the shuffle seed decides which test is first).
func TestLoadInsideASynctestBubbleDatesTheFixturesByTheRealClock(t *testing.T) {
	cipher := testhelper.Cipher(t)
	url := testhelper.EmptyDBURL(t)
	before := time.Now()

	synctest.Test(t, func(t *testing.T) {
		sqlDB, err := sql.Open("pgx", url)
		require.NoError(t, err)
		t.Cleanup(func() { cleanup.Close(sqlDB, "sqlDB") })
		_, err = db.Migrate(t.Context(), sqlDB)
		require.NoError(t, err)

		require.NoError(t, fixtures.Load(t.Context(), sqlDB, cipher, filepath.Join("..", "..", "fixtures")))

		row, err := entclient.Core.New(sqlDB).AuthAttempt.Get(t.Context(), fixtures.FreshLoginAttemptID)
		require.NoError(t, err)
		require.WithinDuration(t, before, row.LastAttemptAt, time.Minute, "the fixture is dated 'now', by the real clock")
	})
}

// staleTombstoneUserID is the User of the tombstone testdata/stale_tombstone holds.
const staleTombstoneUserID = 100001

// A reload replaces the Users with the fixture ones and moves the users sequence back to 100000, so
// the next User created gets an id a User deleted before the reload already had. A tombstone of that
// deleted User left in place makes the deletion of the new one fail on user_tombstones_user_id_key
// (the nightly unverified-users prune on a reseeded dev database), so the reload clears them.
func TestLoadClearsTheTombstonesOfTheUsersItReplaces(t *testing.T) {
	cipher := testhelper.Cipher(t)
	sqlDB, err := sql.Open("pgx", testhelper.EmptyDBURL(t))
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(sqlDB, "sqlDB") })
	_, err = db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)
	require.NoError(t, fixtures.Load(t.Context(), sqlDB, cipher, filepath.Join("testdata", "stale_tombstone")))

	require.NoError(t, fixtures.Load(t.Context(), sqlDB, cipher, filepath.Join("..", "..", "fixtures")))

	stale, err := entclient.Core.New(sqlDB).UserTombstone.Query().
		Where(usertombstone.UserID(staleTombstoneUserID)).Exist(t.Context())
	require.NoError(t, err)
	require.False(t, stale, "the tombstone of a User deleted before the reload is gone")
}
