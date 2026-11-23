package db_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The old unique index held one Enrollment per Contact and Automation, so an existing Enrollment is
// its Contact's first by its beginning: it takes the ordinal 1. An existing version takes the
// default Entry policy, `once` with an hour's cooldown (ADR 0127).
func TestTheEntryPolicyMigrationNumbersAnExistingEnrollmentFirst(t *testing.T) {
	sqlDB := testhelper.SchemaBefore(t, "automation_entry_policy", filepath.Join("testdata", "automation_entry_policy"))

	_, err := db.Migrate(t.Context(), sqlDB)

	require.NoError(t, err)
	c := poolClient(sqlDB)
	run, err := c.AutomationRun.Get(t.Context(), 1)
	require.NoError(t, err)
	version, err := c.AutomationVersion.Get(t.Context(), 1)
	require.NoError(t, err)
	assert.Equal(t, 1, run.Ordinal)
	assert.Nil(t, run.KeyValue)
	assert.Equal(t, automationversion.EntryPolicyOnce, version.EntryPolicy)
	assert.Equal(t, 3600, version.CooldownSeconds)
}
