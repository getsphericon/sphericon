package retention_test

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ee/retention"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func licenseFor(t *testing.T, features ...licensekey.Feature) *licensekey.License {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	key, err := licensekey.Issue(priv, nil, features...)
	require.NoError(t, err)
	lic, err := licensekey.Parse(key, pub, time.Now())
	require.NoError(t, err)
	return lic
}

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
	lic := licenseFor(t, licensekey.FeatureRetention)

	n, err := retention.Prune(t.Context(), env.DB, lic, febFirst)

	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Zero(t, countEntries(t, env, fixtures.AcmeID))
	assert.Equal(t, 5, countEntries(t, env, fixtures.GlobexID), "a Workspace with no retention keeps its log")
}

func TestPruneKeepsEntriesInsideTheWindow(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetRetentionDays(60).ExecX(t.Context())

	n, err := retention.Prune(t.Context(), env.DB, licenseFor(t, licensekey.FeatureRetention), febFirst)

	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Equal(t, 1, countEntries(t, env, fixtures.AcmeID))
}

func TestPrunesNothingWithoutTheRetentionLicense(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetRetentionDays(7).ExecX(t.Context())

	for _, lic := range []*licensekey.License{nil, licenseFor(t), licenseFor(t, licensekey.FeatureAudit)} {
		n, err := retention.Prune(t.Context(), env.DB, lic, febFirst)
		require.NoError(t, err)
		assert.Zero(t, n)
	}
	assert.Equal(t, 1, countEntries(t, env, fixtures.AcmeID))
}

func TestPruneSkipsAWorkspaceInItsDeletionWindow(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetRetentionDays(7).SetDiscardedAt(febFirst).ExecX(t.Context())
	before := countEntries(t, env, fixtures.AcmeID)

	n, err := retention.Prune(t.Context(), env.DB, licenseFor(t, licensekey.FeatureRetention), febFirst)

	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Equal(t, before, countEntries(t, env, fixtures.AcmeID))
}
