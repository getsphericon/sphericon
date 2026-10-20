package external_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/exportstream"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// readExportErasures reads one page of the Erasures stream and checks every line against
// the TypeSpec row schema.
func readExportErasures(t *testing.T, c *externalapi.Client, cursor string) exportPage {
	t.Helper()
	params := externalapi.ExportErasuresParams{}
	if cursor != "" {
		params.Cursor = externalapi.NewOptString(cursor)
	}
	res, err := c.ExportErasures(context.Background(), params)
	require.NoError(t, err)
	ok, isOK := res.(*externalapi.ExportErasuresOKHeaders)
	require.Truef(t, isOK, "got %T", res)
	body, err := io.ReadAll(ok.Response)
	require.NoError(t, err)

	page := exportPage{cursor: ok.XNextCursor}
	for line := range bytes.SplitSeq(bytes.TrimSuffix(body, []byte("\n")), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		require.NoError(t, exportstream.ValidateRow(exportstream.ErasuresSchema, line), string(line))
		var row map[string]any
		require.NoError(t, json.Unmarshal(line, &row))
		page.rows = append(page.rows, row)
	}
	return page
}

// hexSHA256 is the documented hash of a key: lowercase hex of SHA-256.
func hexSHA256(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// rowWithEmailHash finds the row that matched on an address, by its hash.
func rowWithEmailHash(p exportPage, address string) map[string]any {
	for _, r := range p.rows {
		if r["email_sha256"] == hexSHA256(address) {
			return r
		}
	}
	return nil
}

func TestExportErasuresAnnouncesAnErasureByContact(t *testing.T) {
	env := testhelper.Setup(t)
	eraseErasable(t, env)
	env.DeliverToEE(t)

	page := readExportErasures(t, exportClient(t, env, fixtures.InitechID), "")

	row := rowWithEmailHash(page, "erin@initech.test")
	require.NotNil(t, row, "the Erasure left a row")
	assert.Equal(t, "300", row["contact_id"])
	assert.Equal(t, "user-erin-300", row["subject_id"])
	assert.Equal(t, []any{"vis-erasable-laptop"}, row["visitor_ids"])
	assert.Equal(t, hexSHA256("+15550100300"), row["phone_sha256"])
	assert.NotEmpty(t, row["erased_at"])
}

func TestExportErasuresNeverCarryTheAddressOrThePhone(t *testing.T) {
	env := testhelper.Setup(t)
	eraseErasable(t, env)
	env.DeliverToEE(t)

	rows, err := env.DB.Erasure.Query().All(context.Background())
	require.NoError(t, err)
	stored, err := json.Marshal(rows)
	require.NoError(t, err)

	assert.NotContains(t, string(stored), "erin@initech.test")
	assert.NotContains(t, string(stored), "15550100300")
}

func TestExportErasuresAnnouncesAnErasureByAddressAlone(t *testing.T) {
	env := testhelper.Setup(t)
	olga := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)
	res, err := olga.SiteContactsErase(context.Background(), siteapi.SiteContactsEraseParams{
		WorkspaceId: fixtures.InitechPublicID,
		Email:       " " + fixtures.OutboundMessageStrangerDestination + " ",
	})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteContactsEraseNoContent{}, res)
	env.DeliverToEE(t)

	page := readExportErasures(t, exportClient(t, env, fixtures.InitechID), "")

	row := rowWithEmailHash(page, fixtures.OutboundMessageStrangerDestination)
	require.NotNil(t, row, "an address that matched no Contact still yields a row")
	assert.Nil(t, row["contact_id"])
	assert.Nil(t, row["phone_sha256"])
	assert.Empty(t, row["subject_id"], "the address is not echoed as a subject id")
	assert.Equal(t, []any{}, row["visitor_ids"])
}

func TestContactErasedMessageHoldsNoAddressOrPhone(t *testing.T) {
	env := testhelper.Setup(t)
	eraseErasable(t, env)

	envelopes := env.OutboxEnvelopes(t, events.NameContactErased)

	require.Len(t, envelopes, 1)
	body := string(envelopes[0].Data)
	assert.NotContains(t, body, "erin@initech.test")
	assert.NotContains(t, body, "15550100300")
	assert.Contains(t, body, hexSHA256("erin@initech.test"))
}

func TestContactErasedMessageOfAnAddressAloneHoldsNoAddress(t *testing.T) {
	env := testhelper.Setup(t)
	olga := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)
	res, err := olga.SiteContactsErase(context.Background(), siteapi.SiteContactsEraseParams{
		WorkspaceId: fixtures.InitechPublicID,
		Email:       fixtures.OutboundMessageStrangerDestination,
	})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteContactsEraseNoContent{}, res)

	envelopes := env.OutboxEnvelopes(t, events.NameContactErased)

	require.Len(t, envelopes, 1)
	assert.NotContains(t, string(envelopes[0].Data), fixtures.OutboundMessageStrangerDestination)
}

func TestExportErasuresRedeliveryWritesOneRow(t *testing.T) {
	env := testhelper.Setup(t)
	eraseErasable(t, env)
	before, err := env.DB.Erasure.Query().Count(context.Background())
	require.NoError(t, err)

	env.DeliverToEE(t)
	env.DeliverToEE(t)

	after, err := env.DB.Erasure.Query().Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, before+1, after)
}

func TestExportErasuresUnlicensedStoresNothing(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	before, err := env.DB.Erasure.Query().Count(context.Background())
	require.NoError(t, err)
	eraseErasable(t, env)

	env.DeliverToEE(t)

	after, err := env.DB.Erasure.Query().Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestExportErasuresAnswers404WithoutTheLicense(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())

	res, err := exportClient(t, env, fixtures.InitechID).ExportErasures(context.Background(), externalapi.ExportErasuresParams{})

	require.NoError(t, err)
	assert.IsType(t, &externalapi.ExportErasuresNotFound{}, res)
}

func TestExportErasuresNeedTheExportReadScope(t *testing.T) {
	env := testhelper.Setup(t)
	for _, scopes := range [][]string{{}, {"contacts:erase"}, {"contacts:read", "events:read", "audit:read"}} {
		c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.InitechID, scopes...))
		res, err := c.ExportErasures(context.Background(), externalapi.ExportErasuresParams{})
		require.NoError(t, err)
		assert.IsType(t, &externalapi.ExportErasuresUnauthorized{}, res, "scopes %v are refused", scopes)
	}
}

func TestExportErasuresRefuseAMalformedCursor(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := exportClient(t, env, fixtures.InitechID).ExportErasures(context.Background(), externalapi.ExportErasuresParams{
		Cursor: externalapi.NewOptString("not-a-cursor"),
	})

	require.NoError(t, err)
	assert.IsType(t, &externalapi.ExportErasuresBadRequest{}, res)
}

func TestExportErasuresNeverReadAnotherWorkspace(t *testing.T) {
	env := testhelper.Setup(t)

	acme := readExportErasures(t, exportClient(t, env, fixtures.AcmeID), "")
	globex := readExportErasures(t, exportClient(t, env, fixtures.GlobexID), "")

	require.Len(t, globex.rows, 1)
	assert.Equal(t, "900", globex.rows[0]["id"])
	assert.Nil(t, acme.row(900), "Acme never sees Globex's row")
}

func TestExportErasuresReadFromACursorAndReturnTheSameCursorWhenCaughtUp(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.AcmeID)
	first := readExportErasures(t, c, "")
	require.NotEmpty(t, first.rows, "the fixtures hold Acme's Erasures")

	caughtUp := readExportErasures(t, c, first.cursor)

	assert.Empty(t, caughtUp.rows)
	assert.Equal(t, first.cursor, caughtUp.cursor)
}
