package export_test

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/export"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent/erasure"
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

// The Event retention default (ADR 0019): the fixtures hold one Erasure 500 days old and
// one 3 days old in Acme.
const window = 400 * 24 * time.Hour

func acmeHas(t *testing.T, env *testhelper.TestEnv, id int64) bool {
	t.Helper()
	exists, err := env.DB.Scoped(fixtures.AcmeID).Erasure().Query().Where(erasure.ID(id)).Exist(t.Context())
	require.NoError(t, err)
	return exists
}

func TestPruneRemovesErasuresOlderThanTheEventRetentionWindow(t *testing.T) {
	env := testhelper.Setup(t)
	lic := licenseFor(t, licensekey.FeatureExport)

	removed, err := export.Prune(t.Context(), env.DB, lic, window, time.Now())

	require.NoError(t, err)
	assert.Equal(t, 1, removed)
	assert.False(t, acmeHas(t, env, fixtures.AcmeOldErasureID), "past the window")
	assert.True(t, acmeHas(t, env, fixtures.AcmeRecentErasureID), "inside the window")
}

func TestPruneKeepsEveryErasureWhenRetentionIsOff(t *testing.T) {
	env := testhelper.Setup(t)
	lic := licenseFor(t, licensekey.FeatureExport)

	removed, err := export.Prune(t.Context(), env.DB, lic, 0, time.Now().AddDate(10, 0, 0))

	require.NoError(t, err)
	assert.Zero(t, removed)
	assert.True(t, acmeHas(t, env, fixtures.AcmeOldErasureID))
}

func TestPruneWithoutTheExportLicenseRemovesNothing(t *testing.T) {
	env := testhelper.Setup(t)

	removed, err := export.Prune(t.Context(), env.DB, licenseFor(t, licensekey.FeatureAudit), window, time.Now())

	require.NoError(t, err)
	assert.Zero(t, removed)
	assert.True(t, acmeHas(t, env, fixtures.AcmeOldErasureID))
}
