package external_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/licensekey"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/exportstream"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// readExportAudit reads one page of the Audit stream and checks every line against the
// TypeSpec row schema.
func readExportAudit(t *testing.T, c *externalapi.Client, cursor string, limit int32) exportPage {
	t.Helper()
	params := externalapi.ExportAuditParams{}
	if cursor != "" {
		params.Cursor = externalapi.NewOptString(cursor)
	}
	if limit > 0 {
		params.Limit = externalapi.NewOptInt32(limit)
	}
	res, err := c.ExportAudit(context.Background(), params)
	require.NoError(t, err)
	ok, isOK := res.(*externalapi.ExportAuditOKHeaders)
	require.Truef(t, isOK, "got %T", res)
	body, err := io.ReadAll(ok.Response)
	require.NoError(t, err)

	page := exportPage{cursor: ok.XNextCursor}
	for line := range bytes.SplitSeq(bytes.TrimSuffix(body, []byte("\n")), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		require.NoError(t, exportstream.ValidateRow(exportstream.AuditSchema, line), string(line))
		var row map[string]any
		require.NoError(t, json.Unmarshal(line, &row))
		page.rows = append(page.rows, row)
	}
	return page
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

	var seen []int64
	cursor := ""
	for range len(whole.rows) + 1 {
		page := readExportAudit(t, c, cursor, 2)
		if len(page.rows) == 0 {
			break
		}
		seen = append(seen, page.ids()...)
		cursor = page.cursor
	}

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
	for _, scopes := range [][]string{{}, {"audit:read"}} {
		c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.GlobexID, scopes...))
		res, err := c.ExportAudit(context.Background(), externalapi.ExportAuditParams{})
		require.NoError(t, err)
		assert.IsType(t, &externalapi.ExportAuditUnauthorized{}, res, "scopes %v are refused", scopes)
	}
}

func TestExportAuditAnswers404WithoutTheExportLicense(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	c := exportClient(t, env, fixtures.GlobexID)

	res, err := c.ExportAudit(context.Background(), externalapi.ExportAuditParams{})

	require.NoError(t, err)
	assert.IsType(t, &externalapi.ExportAuditNotFound{}, res)
}

func TestExportAuditAnswers404WithoutTheAuditFeature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	key, err := licensekey.Issue(priv, nil, licensekey.FeatureExport)
	require.NoError(t, err)
	env := testhelper.Setup(t, testhelper.WithLicenseKey(pub, key))
	c := exportClient(t, env, fixtures.GlobexID)

	events, err := c.ExportEvents(context.Background(), externalapi.ExportEventsParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ExportEventsOKHeaders{}, events, "export alone still serves the Events stream")

	res, err := c.ExportAudit(context.Background(), externalapi.ExportAuditParams{})

	require.NoError(t, err)
	assert.IsType(t, &externalapi.ExportAuditNotFound{}, res)
}

func TestExportAuditRefusesAMalformedCursor(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.GlobexID)

	res, err := c.ExportAudit(context.Background(), externalapi.ExportAuditParams{
		Cursor: externalapi.NewOptString("not-a-cursor"),
	})

	require.NoError(t, err)
	assert.IsType(t, &externalapi.ExportAuditBadRequest{}, res)
}
