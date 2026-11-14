package auditlogretention_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ee/usecase/auditlogretention"
	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The fixtures hold one entry per Workspace, on 2026-01-01 (Acme) and 2026-01-02 (Globex).
var febFirst = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

func countEntries(t *testing.T, env *testhelper.TestEnv, ws int64) int {
	t.Helper()
	n, err := env.DB.Scoped(ws).AuditEntry().Query().Count(t.Context())
	require.NoError(t, err)
	return n
}

func TestPruneRemovesEntriesOlderThanTheWorkspaceRetention(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetRetentionDays(7).ExecX(t.Context())
	lic := testhelper.License(t, licensekey.FeatureRetention)

	n, err := auditlogretention.New(env.DB, lic, func() time.Time { return febFirst }).Prune(t.Context())

	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Zero(t, countEntries(t, env, fixtures.AcmeID))
	assert.Equal(t, 5, countEntries(t, env, fixtures.GlobexID), "a Workspace with no retention keeps its log")
}

func TestPruneKeepsEntriesInsideTheWindow(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetRetentionDays(60).ExecX(t.Context())

	n, err := auditlogretention.New(env.DB, testhelper.License(t, licensekey.FeatureRetention), func() time.Time { return febFirst }).Prune(t.Context())

	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Equal(t, 1, countEntries(t, env, fixtures.AcmeID))
}

func TestPrunesNothingWithoutTheRetentionLicense(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetRetentionDays(7).ExecX(t.Context())

	for _, lic := range []*licensekey.License{nil, testhelper.License(t), testhelper.License(t, licensekey.FeatureAudit)} {
		n, err := auditlogretention.New(env.DB, lic, func() time.Time { return febFirst }).Prune(t.Context())
		require.NoError(t, err)
		assert.Zero(t, n)
	}
	assert.Equal(t, 1, countEntries(t, env, fixtures.AcmeID))
}

func TestPruneSkipsAWorkspaceInItsDeletionWindow(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.BluthID).SetRetentionDays(7).ExecX(t.Context())
	before := countEntries(t, env, fixtures.BluthID)
	require.Positive(t, before, "BluthAuditEntry is older than the window")

	n, err := auditlogretention.New(env.DB, testhelper.License(t, licensekey.FeatureRetention), func() time.Time { return febFirst }).Prune(t.Context())

	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Equal(t, before, countEntries(t, env, fixtures.BluthID))
}

// An entry older than the client window loses the client's address and user agent and keeps the
// rest; a younger one keeps them. It needs no license: the entries were written under one.
func TestForgetClientsClearsTheAddressOfEntriesOlderThanTheWindow(t *testing.T) {
	env := testhelper.Setup(t)
	// 2026-06-01 less 90 days is 2026-03-03: the February and March entries are older.
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	n, err := auditlogretention.New(env.DB, nil, func() time.Time { return now }).ForgetClients(t.Context())

	require.NoError(t, err)
	assert.Equal(t, 2, n)
	log := env.DB.Scoped(fixtures.GlobexID).AuditEntry()
	old := log.Query().Where(auditentry.ID(fixtures.GlobexAuditIntegrationUpdateID)).OnlyX(t.Context())
	assert.Nil(t, old.IP)
	assert.Equal(t, "req-a", *old.RequestID, "the rest of the entry stays")
	young := log.Query().Where(auditentry.ID(fixtures.GlobexAuditIntegrationDeleteID)).OnlyX(t.Context())
	assert.Equal(t, "10.0.0.1", *young.IP)
}
