package accounts_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const unverifiedRetention = 7 * 24 * time.Hour

func TestPruneUnverifiedDeletesInBoundedBatches(t *testing.T) {
	env := testhelper.Setup(t)
	stale := []int64{fixtures.StaleUnverifiedUserID, fixtures.StaleUnverifiedBulkOneID, fixtures.StaleUnverifiedBulkTwoID}

	// A batch of one forces several rounds; every stale row still goes.
	deleted, err := accounts.New(env.DB, env.Bus).PruneUnverified(t.Context(), unverifiedRetention, 1)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, int(deleted), len(stale), "at least the stale Users go, one batch at a time")
	assert.Zero(t, env.DB.User.Query().Where(user.IDIn(stale...)).CountX(t.Context()))
	assert.True(t, env.DB.User.Query().Where(user.ID(fixtures.FreshUnverifiedUserID)).ExistX(t.Context()), "younger than the window")
}
