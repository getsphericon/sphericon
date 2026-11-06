package auditlogretention_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ee/usecase/auditlogretention"
	entusage "github.com/getsphericon/sphericon/ent/usage"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestPruneNeverRemovesAClosedUsageSnapshot(t *testing.T) {
	env := testhelper.Setup(t)
	before, err := env.DB.Scoped(fixtures.AcmeID).Usage().Query().Where(entusage.FinalizedAtNotNil()).Count(t.Context())
	require.NoError(t, err)
	require.Positive(t, before, "the fixture snapshot of December 2025 is closed")
	env.DB.Workspace.Update().SetRetentionDays(1).ExecX(t.Context())

	n, err := auditlogretention.Prune(t.Context(), env.DB, licenseFor(t, licensekey.FeatureRetention), febFirst.AddDate(5, 0, 0))

	require.NoError(t, err)
	assert.Positive(t, n, "the prune ran and removed the old Audit entries")
	after, err := env.DB.Scoped(fixtures.AcmeID).Usage().Query().Where(entusage.FinalizedAtNotNil()).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, before, after, "snapshots outlive the retention window")
	assert.True(t, env.DB.Usage.Query().Where(entusage.ID(fixtures.AcmeUsageSnapshotID)).ExistX(t.Context()))
}
