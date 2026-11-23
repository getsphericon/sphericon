package db_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A version's stored exit and goal events become Event matchers without a filter (ADR 0133), in
// their order; a version with neither keeps none.
func TestTheEndingMatchersMigrationKeepsEveryExitAndGoalEvent(t *testing.T) {
	sqlDB := testhelper.SchemaBefore(t, "automation_ending_matchers", filepath.Join("testdata", "automation_ending_matchers"))

	_, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)

	versions, err := poolClient(sqlDB).AutomationVersion.Query().Order(ent.Asc(automationversion.FieldVersion)).All(t.Context())
	require.NoError(t, err)
	require.Len(t, versions, 2)
	assert.Equal(t, []eventmatcher.Matcher{{Action: "account_closed"}, {Action: "plan_downgraded"}}, versions[0].Exits)
	assert.Equal(t, &eventmatcher.Matcher{Action: "payment_succeeded"}, versions[0].Goal)
	assert.Empty(t, versions[1].Exits)
	assert.Nil(t, versions[1].Goal)
}
