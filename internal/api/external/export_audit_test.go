package external_test

import (
	"context"
	"crypto/ed25519"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/licensekey"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func readExportAudit(t *testing.T, c *externalapi.Client, cursor string, limit int32) exportPage {
	t.Helper()
	return readExportStream(t, c, auditStream, cursor, limit)
}

func TestExportAuditReadsOldestFirst(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.GlobexID)

	page := readExportAudit(t, c, "", 0)

	want := []int64{
		fixtures.GlobexAuditEntryID,
		fixtures.GlobexAuditIntegrationUpdateID,
		fixtures.GlobexAuditWebhookCreateID,
		fixtures.GlobexAuditIntegrationDeleteID,
		fixtures.GlobexAuditFormulaEntryID,
	}
	assert.Equal(t, want, page.ids())
	assert.NotEmpty(t, page.cursor)
	first := page.row(fixtures.GlobexAuditEntryID)
	assert.Equal(t, fixtures.GlobexAuditEntryAction, first["action"])
	assert.Equal(t, "Jane", first["actor_name"])
	assert.Equal(t, map[string]any{"role": map[string]any{"from": "admin", "to": "owner"}}, first["diff"])
	assert.Equal(t, map[string]any{}, page.row(fixtures.GlobexAuditIntegrationUpdateID)["diff"], "an absent diff is an empty object")
	assert.Nil(t, page.row(fixtures.GlobexAuditIntegrationUpdateID)["actor_name"])
}

func TestExportAuditPagesWithoutLosingARow(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.GlobexID)
	whole := readExportAudit(t, c, "", 0)

	seen, cursor := pageThrough(func(cursor string, limit int32) exportPage { return readExportAudit(t, c, cursor, limit) }, 2, len(whole.rows)+1)

	assert.Equal(t, whole.ids(), seen)
	caughtUp := readExportAudit(t, c, cursor, 2)
	assert.Empty(t, caughtUp.rows)
	assert.Equal(t, cursor, caughtUp.cursor)
}

func TestExportAuditShowsAStaffMemberAsSphericonStaff(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.InitechID)

	page := readExportAudit(t, c, "", 0)

	staff := page.row(fixtures.InitechAuditStaffSuspendID)
	require.NotNil(t, staff)
	assert.Equal(t, "staff", staff["actor_kind"])
	assert.Equal(t, "Sphericon staff", staff["actor_name"])
	assert.Nil(t, staff["actor_id"], "the Staff member's identity is never exposed")
}

func TestExportAuditNeverReadsAnotherWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	initech := readExportAudit(t, exportClient(t, env, fixtures.InitechID), "", 0)

	assert.NotNil(t, initech.row(fixtures.InitechAuditUserTagID))
	assert.Nil(t, initech.row(fixtures.GlobexAuditEntryID))
	assert.Nil(t, initech.row(fixtures.AcmeAuditEntryID))
}

func TestExportAuditNeedsTheExportReadScope(t *testing.T) {
	env := testhelper.Setup(t)
	for _, scopes := range [][]apitokens.Scope{{}, {apitokens.ScopeAuditRead}} {
		c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.GlobexID, scopes...))
		_, err := c.ExportAudit(context.Background(), externalapi.ExportAuditParams{})
		testhelper.ExternalRefusal(t, err, http.StatusForbidden, "scopes %v are refused", scopes)
	}
}

func TestExportAuditAnswers404WithoutTheExportLicense(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	c := exportClient(t, env, fixtures.GlobexID)

	_, err := c.ExportAudit(context.Background(), externalapi.ExportAuditParams{})

	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
}

func TestExportAuditAnswers404WithoutTheAuditFeature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	key, err := licensekey.Issue(priv, nil, licensekey.FeatureExport)
	require.NoError(t, err)
	env := testhelper.Setup(t, testhelper.WithLicenseKey(pub, key))
	c := exportClient(t, env, fixtures.GlobexID)

	_, err = c.ExportEvents(context.Background(), externalapi.ExportEventsParams{})
	require.NoError(t, err)

	_, err = c.ExportAudit(context.Background(), externalapi.ExportAuditParams{})

	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
}

func TestExportAuditRefusesAMalformedCursor(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.GlobexID)

	_, err := c.ExportAudit(context.Background(), externalapi.ExportAuditParams{
		Cursor: externalapi.NewOptString("not-a-cursor"),
	})

	testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
}
