package external_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func readExportErasures(t *testing.T, c *externalapi.Client, cursor string) exportPage {
	t.Helper()
	return readExportStream(t, c, erasuresStream, cursor, 0)
}

// hexSHA256 is the documented hash of a key: lowercase hex of SHA-256.
func hexSHA256(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// rowWithEmailHash finds the row that matched on an address, by its hash.
func rowWithEmailHash(p exportPage, address string) map[string]any {
	for _, r := range p.rows {
		if r["email_hash"] == hexSHA256(address) {
			return r
		}
	}
	return nil
}

func TestExportErasuresAnnouncesAnErasureByContact(t *testing.T) {
	env := testhelper.Setup(t)
	eraseErasable(t, env)
	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)

	page := readExportErasures(t, exportClient(t, env, fixtures.InitechID), "")

	row := rowWithEmailHash(page, "erin@initech.test")
	require.NotNil(t, row, "the Erasure left a row")
	assert.Equal(t, "300", row["contact_id"])
	assert.Equal(t, "user-erin-300", row["subject_id"])
	assert.Equal(t, []any{"vis-erasable-laptop"}, row["visitor_ids"])
	assert.Equal(t, hexSHA256("+14155550300"), row["phone_hash"])
	assert.NotEmpty(t, row["erased_at"])
}

func TestExportErasuresNeverCarryTheAddressOrThePhone(t *testing.T) {
	env := testhelper.Setup(t)
	eraseErasable(t, env)
	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)

	rows, err := env.DB.Erasure.Query().All(context.Background())
	require.NoError(t, err)
	stored, err := json.Marshal(rows)
	require.NoError(t, err)

	assert.NotContains(t, string(stored), "erin@initech.test")
	assert.NotContains(t, string(stored), "14155550300")
}

func TestExportErasuresAnnouncesAnErasureByAddressAlone(t *testing.T) {
	env := testhelper.Setup(t)
	olga := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)
	err := olga.SiteContactsErase(context.Background(), siteapi.SiteContactsEraseParams{
		WorkspaceId: fixtures.InitechPublicID,
		Email:       " " + fixtures.OutboundMessageStrangerDestination + " ",
	})
	require.NoError(t, err)
	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)

	page := readExportErasures(t, exportClient(t, env, fixtures.InitechID), "")

	row := rowWithEmailHash(page, fixtures.OutboundMessageStrangerDestination)
	require.NotNil(t, row, "an address that matched no Contact still yields a row")
	assert.Nil(t, row["contact_id"])
	assert.Nil(t, row["phone_hash"])
	assert.Empty(t, row["subject_id"], "the address is not echoed as a subject id")
	assert.Equal(t, []any{}, row["visitor_ids"])
}

func TestContactErasedMessageHoldsNoAddressOrPhone(t *testing.T) {
	env := testhelper.Setup(t)
	eraseErasable(t, env)

	envelopes := env.QueuedEnvelopes(t, events.NameContactErased)

	require.Len(t, envelopes, 1)
	body := string(envelopes[0].Data)
	assert.NotContains(t, body, "erin@initech.test")
	assert.NotContains(t, body, "14155550300")
	assert.Contains(t, body, hexSHA256("erin@initech.test"))
}

func TestContactErasedMessageOfAnAddressAloneHoldsNoAddress(t *testing.T) {
	env := testhelper.Setup(t)
	olga := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)
	err := olga.SiteContactsErase(context.Background(), siteapi.SiteContactsEraseParams{
		WorkspaceId: fixtures.InitechPublicID,
		Email:       fixtures.OutboundMessageStrangerDestination,
	})
	require.NoError(t, err)

	envelopes := env.QueuedEnvelopes(t, events.NameContactErased)

	require.Len(t, envelopes, 1)
	assert.NotContains(t, string(envelopes[0].Data), fixtures.OutboundMessageStrangerDestination)
}

func TestExportErasuresRedeliveryWritesOneRow(t *testing.T) {
	env := testhelper.Setup(t)
	eraseErasable(t, env)
	before, err := env.DB.Erasure.Query().Count(context.Background())
	require.NoError(t, err)

	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)
	env.RetryJobs(t, testhelper.EditionJobs...)
	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)

	after, err := env.DB.Erasure.Query().Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, before+1, after)
}

func TestExportErasuresUnlicensedStoresNothing(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	before, err := env.DB.Erasure.Query().Count(context.Background())
	require.NoError(t, err)
	eraseErasable(t, env)

	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)

	after, err := env.DB.Erasure.Query().Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestExportErasuresAnswers404WithoutTheLicense(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())

	_, err := exportClient(t, env, fixtures.InitechID).ExportErasures(context.Background(), externalapi.ExportErasuresParams{})

	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
}

func TestExportErasuresNeedTheExportReadScope(t *testing.T) {
	env := testhelper.Setup(t)
	for _, scopes := range [][]string{{}, {"contacts:erase"}, {"contacts:read", "events:read", "audit:read"}} {
		c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.InitechID, scopes...))
		_, err := c.ExportErasures(context.Background(), externalapi.ExportErasuresParams{})
		testhelper.ExternalRefusal(t, err, http.StatusForbidden, "scopes %v are refused", scopes)
	}
}

func TestExportErasuresRefuseAMalformedCursor(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := exportClient(t, env, fixtures.InitechID).ExportErasures(context.Background(), externalapi.ExportErasuresParams{
		Cursor: externalapi.NewOptString("not-a-cursor"),
	})

	testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
}

func TestExportErasuresNeverReadAnotherWorkspace(t *testing.T) {
	env := testhelper.Setup(t)

	acme := readExportErasures(t, exportClient(t, env, fixtures.AcmeID), "")
	globex := readExportErasures(t, exportClient(t, env, fixtures.GlobexID), "")

	require.Len(t, globex.rows, 1)
	assert.Equal(t, "900", globex.rows[0]["id"])
	assert.Nil(t, acme.row(900), "Acme never sees Globex's row")
}
