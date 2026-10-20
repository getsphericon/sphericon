package external_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/exportstream"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// readExportMessages reads one page of the Outbound messages stream and checks every line
// against the TypeSpec row schema.
func readExportMessages(t *testing.T, c *externalapi.Client, cursor string) exportPage {
	t.Helper()
	params := externalapi.ExportMessagesParams{}
	if cursor != "" {
		params.Cursor = externalapi.NewOptString(cursor)
	}
	res, err := c.ExportMessages(context.Background(), params)
	require.NoError(t, err)
	ok, isOK := res.(*externalapi.ExportMessagesOKHeaders)
	require.Truef(t, isOK, "got %T", res)
	body, err := io.ReadAll(ok.Response)
	require.NoError(t, err)

	page := exportPage{cursor: ok.XNextCursor}
	for line := range bytes.SplitSeq(bytes.TrimSuffix(body, []byte("\n")), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		require.NoError(t, exportstream.ValidateRow(exportstream.MessagesSchema, line), string(line))
		var row map[string]any
		require.NoError(t, json.Unmarshal(line, &row))
		page.rows = append(page.rows, row)
	}
	return page
}

func TestExportMessagesReadsDeliveryMetadataWithoutContent(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.InitechID)

	page := readExportMessages(t, c, "")

	require.NotEmpty(t, page.cursor)
	broadcast := page.row(fixtures.OutboundMessageErasableBroadcastID)
	require.NotNil(t, broadcast)
	assert.Equal(t, "broadcast", broadcast["kind"])
	assert.Equal(t, "email", broadcast["channel"])
	assert.Equal(t, fixtures.OutboundMessageErasableBroadcastDestination, broadcast["destination"])
	assert.Equal(t, "sent", broadcast["status"])
	assert.Equal(t, "mail.initech.test", broadcast["sending_domain"])
	assert.NotNil(t, broadcast["contact_id"])
	assert.NotNil(t, broadcast["broadcast_id"])
	assert.NotNil(t, broadcast["sent_at"])
	assert.NotContains(t, broadcast, "subject")
	assert.NotContains(t, broadcast, "body")
	stranger := page.row(fixtures.OutboundMessageStrangerID)
	require.NotNil(t, stranger)
	assert.Nil(t, stranger["contact_id"])
}

func TestExportMessagesDeliversAStatusChangeAgain(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.GlobexID)
	before := readExportMessages(t, c, "")
	require.Equal(t, "pending", before.row(fixtures.OutboundMessageGlobexID)["status"])

	require.NoError(t, env.DB.Scoped(fixtures.GlobexID).OutboundMessage().
		UpdateOneID(fixtures.OutboundMessageGlobexID).
		SetStatus(outboundmessage.StatusSent).
		Exec(context.Background()))

	after := readExportMessages(t, c, before.cursor)
	changed := after.row(fixtures.OutboundMessageGlobexID)
	require.NotNil(t, changed, "the changed row comes again from the cursor taken before")
	assert.Equal(t, "sent", changed["status"])
}

func TestExportMessagesDeliversAnErasedDestinationAnonymized(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.InitechID)
	before := readExportMessages(t, c, "")
	require.Equal(t, fixtures.OutboundMessageErasableBroadcastDestination,
		before.row(fixtures.OutboundMessageErasableBroadcastID)["destination"])

	eraseErasable(t, env)

	after := readExportMessages(t, c, before.cursor)
	for _, id := range []int64{fixtures.OutboundMessageErasableBroadcastID, fixtures.OutboundMessageErasableTransactionalID} {
		row := after.row(id)
		require.NotNilf(t, row, "message %d comes again", id)
		assert.Nil(t, row["destination"])
		assert.Nil(t, row["contact_id"])
	}
	assert.Nil(t, after.row(fixtures.OutboundMessageBystanderID), "an unrelated message is not redelivered")
}

func TestExportMessagesNeedsTheExportReadScope(t *testing.T) {
	env := testhelper.Setup(t)
	for _, scopes := range [][]string{{}, {"contacts:read"}, {"contacts:read", "events:read", "audit:read"}} {
		c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.InitechID, scopes...))
		res, err := c.ExportMessages(context.Background(), externalapi.ExportMessagesParams{})
		require.NoError(t, err)
		assert.IsType(t, &externalapi.ExportMessagesUnauthorized{}, res, "scopes %v are refused", scopes)
	}
}

func TestExportMessagesAnswers404WithoutTheLicense(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	c := exportClient(t, env, fixtures.InitechID)

	res, err := c.ExportMessages(context.Background(), externalapi.ExportMessagesParams{})

	require.NoError(t, err)
	assert.IsType(t, &externalapi.ExportMessagesNotFound{}, res)
}

func TestExportMessagesNeverReadsAnotherWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	globex := readExportMessages(t, exportClient(t, env, fixtures.GlobexID), "")
	initech := readExportMessages(t, exportClient(t, env, fixtures.InitechID), "")

	require.NotNil(t, globex.row(fixtures.OutboundMessageGlobexID))
	assert.Nil(t, initech.row(fixtures.OutboundMessageGlobexID))
	assert.Nil(t, globex.row(fixtures.OutboundMessageErasableBroadcastID))
}

func TestExportMessagesRefuseAMalformedCursor(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.InitechID)

	res, err := c.ExportMessages(context.Background(), externalapi.ExportMessagesParams{
		Cursor: externalapi.NewOptString("not-a-cursor"),
	})

	require.NoError(t, err)
	assert.IsType(t, &externalapi.ExportMessagesBadRequest{}, res)
}
