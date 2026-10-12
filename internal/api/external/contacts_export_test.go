package external_test

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/go-faster/jx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/ent/visitor"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// readExport reads the streamed body to the end and decodes it as the contract's
// ContactExportDocument, so a missing member or a malformed value fails here. It
// also returns the raw body for the checks a typed decode cannot make.
func readExport(t *testing.T, res externalapi.ContactsExportRes) (externalapi.ContactExportDocument, []byte, string) {
	t.Helper()
	ok, isOK := res.(*externalapi.ContactsExportOKApplicationOctetStreamHeaders)
	require.Truef(t, isOK, "got %T", res)
	raw, err := io.ReadAll(ok.Response)
	require.NoError(t, err)
	var doc externalapi.ContactExportDocument
	require.NoError(t, doc.Decode(jx.DecodeBytes(raw)), string(raw))
	require.NoError(t, doc.Validate())
	return doc, raw, ok.ContentDisposition
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

// camel turns a snake_case column name into the contract's camelCase key.
func camel(column string) string {
	parts := strings.Split(column, "_")
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

func TestExternalContactsExportByIDReturnsTheFullBundle(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:read")

	res, err := c.ContactsExport(context.Background(), externalapi.ContactsExportParams{
		ID: externalapi.NewOptEntityId(entityIDString(fixtures.ContactExportSubjectID)),
	})
	require.NoError(t, err)
	doc, _, disposition := readExport(t, res)

	assert.Contains(t, disposition, "attachment")
	assert.Equal(t, fixtures.ContactExportSubjectEmail, doc.Contact.Email.Value)
	assert.Equal(t, "user:export-subject", doc.Contact.SubjectId.Value)
	assert.Equal(t, "Eva", doc.Contact.FirstName.Value)
	assert.JSONEq(t, `{"plan":"pro","seats":4}`, jsonOf(t, doc.Contact.CustomFields.Value))

	tagNames := lo.Map(doc.Tags, func(tg externalapi.ContactExportTag, _ int) string { return tg.Name })
	assert.ElementsMatch(t, []string{"vip", "newsletter"}, tagNames)
	assert.Len(t, doc.Visitors, 2)

	actions := lo.Map(doc.Events, func(e externalapi.ContactExportEvent, _ int) string { return e.Action })
	assert.ElementsMatch(t, []string{"export_subject_custom", "email.sent", "marketing.confirmed"}, actions)

	assert.Len(t, doc.Unsubscribes, 1)
	assert.Len(t, doc.Suppressions, 1)
	assert.Len(t, doc.Confirmations, 1)
	assert.Len(t, doc.OutboundMessages, 2, "includes the send to the address that has no Contact link")
	assert.Len(t, doc.BroadcastRecipients, 1)
}

// The document is the contract's typed projection, not the stored rows. Goverter
// guarantees every DTO field has a source; it cannot notice a stored column that
// has no DTO field, which for an Art. 15 export would be silently withheld data. So
// every column of each exported entity is either a field of its DTO or named here as
// deliberately left out: the tenant id (every row is the caller's), the contact
// back-reference (the document is that contact), internal links and keys, and
// updated_at on rows that are written once.
func TestExternalContactsExportMembersCoverEveryStoredColumn(t *testing.T) {
	covered := func(dto reflect.Type, columns []string, left ...string) {
		t.Helper()
		keys := lo.Map(reflect.VisibleFields(dto), func(f reflect.StructField, _ int) string {
			return strings.Split(f.Tag.Get("json"), ",")[0]
		})
		want := lo.Map(lo.Without(columns, left...), func(col string, _ int) string { return camel(col) })
		assert.ElementsMatch(t, want, keys, dto.Name())
	}
	covered(reflect.TypeFor[externalapi.ContactExportContact](), contact.Columns, "workspace_id")
	covered(reflect.TypeFor[externalapi.ContactExportTag](), tag.Columns, "workspace_id", "updated_at")
	covered(reflect.TypeFor[externalapi.ContactExportVisitor](), visitor.Columns, "workspace_id", "contact_id", "updated_at")
	covered(reflect.TypeFor[externalapi.ContactExportEvent](), event.Columns, "workspace_id", "contact_id", "updated_at")
	covered(reflect.TypeFor[externalapi.ContactExportUnsubscribe](), unsubscribe.Columns, "workspace_id", "contact_id", "updated_at")
	covered(reflect.TypeFor[externalapi.ContactExportSuppression](), suppression.Columns, "workspace_id", "contact_id", "updated_at")
	covered(reflect.TypeFor[externalapi.ContactExportConfirmation](), confirmation.Columns, "workspace_id", "contact_id", "updated_at")
	covered(reflect.TypeFor[externalapi.ContactExportOutboundMessage](), outboundmessage.Columns,
		"workspace_id", "contact_id", "updated_at", "idempotency_key", "broadcast_recipient_id", "automation_run_id", "integration_id")
	covered(reflect.TypeFor[externalapi.ContactExportBroadcastRecipient](), broadcastrecipient.Columns,
		"workspace_id", "contact_id", "updated_at", "outbound_message_id", "deferred_until")
}

// No tenant id and no rendered message content reach the document.
func TestExternalContactsExportLeaksNoInternals(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:read")

	res, err := c.ContactsExport(context.Background(), externalapi.ContactsExportParams{
		ID: externalapi.NewOptEntityId(entityIDString(fixtures.ContactExportSubjectID)),
	})
	require.NoError(t, err)
	_, raw, _ := readExport(t, res)

	for _, leaked := range []string{"workspaceId", "workspace_id", `"body"`, `"html"`, "renderedBody"} {
		assert.NotContains(t, string(raw), leaked)
	}
}

func TestExternalContactsExportByEmailMatchesByID(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:read")

	res, err := c.ContactsExport(context.Background(), externalapi.ContactsExportParams{
		Email: externalapi.NewOptEmailAddress("Export.Subject@Example.com"),
	})
	require.NoError(t, err)
	doc, _, _ := readExport(t, res)

	assert.Equal(t, entityIDString(fixtures.ContactExportSubjectID), doc.Contact.ID)
	assert.Len(t, doc.Events, 3)
}

func TestExternalContactsExportEmptyMembersAreArrays(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:read")

	res, err := c.ContactsExport(context.Background(), externalapi.ContactsExportParams{
		ID: externalapi.NewOptEntityId(entityIDString(fixtures.ContactBobID)),
	})
	require.NoError(t, err)
	doc, raw, _ := readExport(t, res)
	assert.Empty(t, doc.Visitors)
	assert.Empty(t, doc.Tags)
	assert.Contains(t, string(raw), `"visitors":[]`, "an empty member is an array, never null")
	assert.Contains(t, string(raw), `"tags":[]`)
	assert.Len(t, doc.Unsubscribes, 1, "bob's broadcasts opt-out belongs to his Destination")
}

func TestExternalContactsExportStreamsLargeContacts(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	const extra = 1200 // more than two pages of the keyset reader
	s := env.DB.Scoped(fixtures.AcmeID)
	bulk := make([]*ent.EventScopedCreate, 0, extra)
	for range extra {
		bulk = append(bulk, s.Event().Create().SetContactID(fixtures.ContactExportSubjectID).SetAction("bulk_event"))
	}
	require.NoError(t, s.Event().CreateBulk(bulk...).Exec(ctx))

	c := env.ExternalScoped(t, "contacts:read")
	res, err := c.ContactsExport(ctx, externalapi.ContactsExportParams{
		ID: externalapi.NewOptEntityId(entityIDString(fixtures.ContactExportSubjectID)),
	})
	require.NoError(t, err)

	// The document stays one valid JSON value across the keyset pages and holds
	// every Event of the Contact.
	doc, _, _ := readExport(t, res)
	assert.Len(t, doc.Events, extra+3)
}

func TestExternalContactsExportNeedsTheReadScope(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.ExternalScoped(t, "contacts:write", "events:read").ContactsExport(context.Background(), externalapi.ContactsExportParams{
		ID: externalapi.NewOptEntityId(entityIDString(fixtures.ContactExportSubjectID)),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsExportUnauthorized{}, res)
}

func TestExternalContactsExportIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:read")
	ctx := context.Background()

	byID, err := c.ContactsExport(ctx, externalapi.ContactsExportParams{
		ID: externalapi.NewOptEntityId(entityIDString(fixtures.ContactGlobexID)),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsExportNotFound{}, byID)

	byEmail, err := c.ContactsExport(ctx, externalapi.ContactsExportParams{
		Email: externalapi.NewOptEmailAddress(fixtures.ContactGlobexEmail),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsExportNotFound{}, byEmail)
}

func TestExternalContactsExportNeedsExactlyOneIdentifier(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:read")
	ctx := context.Background()

	none, err := c.ContactsExport(ctx, externalapi.ContactsExportParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsExportBadRequest{}, none)

	both, err := c.ContactsExport(ctx, externalapi.ContactsExportParams{
		ID:    externalapi.NewOptEntityId(entityIDString(fixtures.ContactExportSubjectID)),
		Email: externalapi.NewOptEmailAddress(fixtures.ContactExportSubjectEmail),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsExportBadRequest{}, both)
}
