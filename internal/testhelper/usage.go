package testhelper

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/audit"
	"github.com/getsphericon/sphericon/ee/export"
	"github.com/getsphericon/sphericon/ee/usage"
	"github.com/getsphericon/sphericon/internal/usecase/workspacedeletion"
)

// RefreshUsage runs the usage job once as of now under the harness license, which
// includes the `staff` license feature: it closes the elapsed Usage periods and refreshes the
// Live usage of the open one, as the periodic job does (the river schedule does not run
// in tests).
func (env *TestEnv) RefreshUsage(t *testing.T, now time.Time) {
	t.Helper()
	lic, err := testLicense()
	require.NoError(t, err)
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, now))
}

// EditionTables are the removals of the Enterprise Edition's own Workspace-owned tables that the
// wiring hands the Workspace deletion use case (internal/app/container.go): a test that builds the
// use case itself erases them too.
func EditionTables() workspacedeletion.EditionTables {
	return workspacedeletion.EditionTables{
		AuditEntries: audit.EraseAuditEntriesIn, Erasures: export.EraseErasuresIn, Usage: usage.EraseUsageIn,
	}
}
