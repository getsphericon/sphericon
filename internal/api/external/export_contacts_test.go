package external_test

import (
	"context"
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func readExportContacts(t *testing.T, c *externalapi.Client, cursor string, limit int32) exportPage {
	t.Helper()
	return readExportStream(t, c, contactsStream, cursor, limit)
}

// exportContactsClient carries the scopes to read the stream and to change a Contact and
// its Tags through the same API a customer uses.
func exportContactsClient(t *testing.T, env *testhelper.TestEnv, workspaceID int64) *externalapi.Client {
	t.Helper()
	return env.ExternalWithToken(t, env.ScopedBearerFor(t, workspaceID, "export:read", "contacts:read", "contacts:write"))
}

func TestExportContactsRowCarriesColumnsCustomFieldsAndTags(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportContactsClient(t, env, fixtures.AcmeID)

	page := readExportContacts(t, c, "", 0)

	alice := page.row(fixtures.ContactAliceID)
	require.NotNil(t, alice)
	assert.Equal(t, "alice@example.com", alice["email"])
	assert.Equal(t, "Alice", alice["first_name"])
	assert.Nil(t, alice["phone"])
	assert.Equal(t, []any{"newsletter", "vip"}, alice["tags"])
	assert.Equal(t, map[string]any{}, alice["custom_fields"])
	liam := page.row(fixtures.ContactLiamID)
	require.NotNil(t, liam)
	assert.Equal(t, "pro", liam["custom_fields"].(map[string]any)["plan"])
	assert.Equal(t, []any{}, liam["tags"])
}

func TestExportContactsDeliversAContactAgainOnACustomFieldEdit(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportContactsClient(t, env, fixtures.AcmeID)
	before := readExportContacts(t, c, "", 0)

	_, err := c.ContactsUpdate(context.Background(), &externalapi.UpdateContactInput{
		CustomFields: externalapi.NewOptNilUpdateContactInputCustomFields(externalapi.UpdateContactInputCustomFields{"plan": jx.Raw(`"team"`)}),
	}, externalapi.ContactsUpdateParams{ID: entityIDString(fixtures.ContactLiamID)})
	require.NoError(t, err)

	after := readExportContacts(t, c, before.cursor, 0)
	liam := after.row(fixtures.ContactLiamID)
	require.NotNil(t, liam, "the edited Contact comes again from the cursor taken before")
	assert.Equal(t, "team", liam["custom_fields"].(map[string]any)["plan"])
}

func TestExportContactsDeliversAContactAgainOnATagChange(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := exportContactsClient(t, env, fixtures.AcmeID)
	before := readExportContacts(t, c, "", 0)

	_, err := c.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "churn-risk"},
		externalapi.TagsApplyParams{ContactId: entityIDString(fixtures.ContactBobID)})
	require.NoError(t, err)

	attached := readExportContacts(t, c, before.cursor, 0)
	bob := attached.row(fixtures.ContactBobID)
	require.NotNil(t, bob, "attaching a Tag re-delivers the Contact")
	assert.Equal(t, []any{"churn-risk"}, bob["tags"])

	_, err = c.TagsRemove(ctx, externalapi.TagsRemoveParams{ContactId: entityIDString(fixtures.ContactBobID), Name: "churn-risk"})
	require.NoError(t, err)

	detached := readExportContacts(t, c, attached.cursor, 0)
	bob = detached.row(fixtures.ContactBobID)
	require.NotNil(t, bob, "detaching a Tag re-delivers the Contact")
	assert.Equal(t, []any{}, bob["tags"])
}

func TestExportContactsPagingLosesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportContactsClient(t, env, fixtures.AcmeID)
	whole := readExportContacts(t, c, "", 0)
	require.Greater(t, len(whole.rows), 3)

	seen, _ := pageThrough(func(cursor string, limit int32) exportPage { return readExportContacts(t, c, cursor, limit) }, 3, len(whole.rows)+1)

	assert.Equal(t, whole.ids(), seen, "every row exactly once, in order")
}

func TestExportContactsNeedsTheExportReadScope(t *testing.T) {
	env := testhelper.Setup(t)
	for _, scopes := range [][]string{{}, {"contacts:read"}, {"contacts:read", "events:read", "audit:read"}} {
		c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.AcmeID, scopes...))
		res, err := c.ExportContacts(context.Background(), externalapi.ExportContactsParams{})
		require.NoError(t, err)
		assert.IsType(t, &externalapi.ExportContactsForbidden{}, res, "scopes %v are refused", scopes)
	}
}

func TestExportContactsAnswers404WithoutTheLicense(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	c := exportContactsClient(t, env, fixtures.AcmeID)

	res, err := c.ExportContacts(context.Background(), externalapi.ExportContactsParams{})

	require.NoError(t, err)
	assert.IsType(t, &externalapi.ExportContactsNotFound{}, res)
}

func TestExportContactsNeverReadsAnotherWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	acme := readExportContacts(t, exportContactsClient(t, env, fixtures.AcmeID), "", 0)
	initech := readExportContacts(t, exportContactsClient(t, env, fixtures.InitechID), "", 0)

	require.NotNil(t, initech.row(fixtures.ContactErasableID))
	assert.Equal(t, []any{"initech-shared"}, initech.row(fixtures.ContactErasableID)["tags"])
	assert.Nil(t, acme.row(fixtures.ContactErasableID))
	assert.Nil(t, initech.row(fixtures.ContactAliceID))
}

func TestExportContactsRefuseAMalformedCursor(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportContactsClient(t, env, fixtures.AcmeID)

	res, err := c.ExportContacts(context.Background(), externalapi.ExportContactsParams{
		Cursor: externalapi.NewOptString("not-a-cursor"),
	})

	require.NoError(t, err)
	assert.IsType(t, &externalapi.ExportContactsBadRequest{}, res)
}
