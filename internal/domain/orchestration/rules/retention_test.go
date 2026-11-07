package rules_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// eventsRetention is the 400-day window the fixture's expired Firing is past.
const eventsRetention = 400 * 24 * time.Hour

func firingPresent(t *testing.T, env *testhelper.TestEnv, id int64) bool {
	t.Helper()
	return env.DB.RuleFiring.Query().Where(rulefiring.ID(id)).ExistX(context.Background())
}

func TestPruneRuleFiringsDeletesInBoundedBatches(t *testing.T) {
	env := testhelper.Setup(t)

	deleted, err := rules.PruneFirings(context.Background(), env.DB, eventsRetention, 1, time.Now())

	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)
	assert.False(t, firingPresent(t, env, fixtures.RuleFiringExpiredID))
}
