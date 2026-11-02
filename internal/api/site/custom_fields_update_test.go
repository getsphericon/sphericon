package site_test

import (
	"context"
	"testing"

	"github.com/go-faster/jx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// writeFields writes one Contact whose custom fields are the raw JSON values by key.
func writeFields(t *testing.T, c *siteapi.Client, email string, values map[string]string) {
	t.Helper()
	fields := siteapi.JsonObject{}
	for key, raw := range values {
		fields[key] = jx.Raw(raw)
	}
	out, err := c.SiteContactsCreate(context.Background(), &siteapi.SiteCreateContactInput{
		Email:        siteapi.NewOptNilEmailAddress(siteapi.EmailAddress(email)),
		CustomFields: siteapi.NewOptNilJsonObject(fields),
	}, siteapi.SiteContactsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteContactResource{}, out)
}

// fieldByKey is the Acme Custom field with the key, read from the catalogue.
func fieldByKey(t *testing.T, c *siteapi.Client, key string) siteapi.SiteCustomFieldResource {
	t.Helper()
	out, err := c.SiteCustomFieldsList(context.Background(), siteapi.SiteCustomFieldsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	list, ok := out.(*siteapi.SiteCustomFieldsListOK)
	require.Truef(t, ok, "got %T", out)
	field, found := lo.Find(list.Items, func(f siteapi.SiteCustomFieldResource) bool { return f.Key == key })
	require.Truef(t, found, "no custom field %q", key)
	return field
}

func updateField(c *siteapi.Client, id siteapi.EntityId, in *siteapi.SiteUpdateCustomFieldInput) (siteapi.SiteCustomFieldsUpdateRes, error) {
	return c.SiteCustomFieldsUpdate(context.Background(), in, siteapi.SiteCustomFieldsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
}

func previewAcme(t *testing.T, c *siteapi.Client, leaf string) int32 {
	t.Helper()
	def := `{"kind":"group","combinator":"and","rules":[` + leaf + `]}`
	out, err := c.SiteSegmentsPreview(context.Background(), &siteapi.SitePreviewSegmentInput{
		Condition: siteapi.NewOptSiteConditionGroup(testhelper.SiteRuleGroup(t, def)),
	}, siteapi.SiteSegmentsPreviewParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	res, ok := out.(*siteapi.SitePreviewSegmentResult)
	require.Truef(t, ok, "got %T", out)
	return res.Count
}

// A Contact write whose first value of a new key is one of the three ISO forms makes a datetime
// field; any other shape, a slash date included, infers as before.
func TestAContactWriteMakesADatetimeFieldFromAnISODate(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	writeFields(t, c, "iso@example.com", map[string]string{
		"d_zulu": `"2026-01-15T10:00:00Z"`, "d_offset": `"2026-01-15T10:00:00.5+03:00"`,
		"d_local": `"2026-01-15T10:00:00"`, "d_bare": `"2026-01-15"`,
		"d_slash": `"01/15/2026"`, "d_text": `"soon"`, "d_number": `1768435200`,
	})

	assert.Equal(t, siteapi.SiteCustomFieldTypeDatetime, fieldByKey(t, c, "d_zulu").Type)
	assert.Equal(t, siteapi.SiteCustomFieldTypeDatetime, fieldByKey(t, c, "d_offset").Type)
	assert.Equal(t, siteapi.SiteCustomFieldTypeDatetime, fieldByKey(t, c, "d_local").Type)
	assert.Equal(t, siteapi.SiteCustomFieldTypeDatetime, fieldByKey(t, c, "d_bare").Type)
	assert.Equal(t, siteapi.SiteCustomFieldTypeString, fieldByKey(t, c, "d_slash").Type)
	assert.Equal(t, siteapi.SiteCustomFieldTypeString, fieldByKey(t, c, "d_text").Type)
	assert.Equal(t, siteapi.SiteCustomFieldTypeNumber, fieldByKey(t, c, "d_number").Type)
}

// A date leaf matches a Contact holding each ISO form, an offset-less one judged in the Workspace
// time zone, and not text or Unix seconds in the field. Against the bound 2026-01-14T23:00:00Z
// (the evening before 2026-01-15), "2026-01-15" is after it in UTC and before it in Tokyo, where
// that day began at 15:00 UTC of the 14th; "2026-01-15T10:00:00" is after it in both.
func TestADateLeafReadsEachISOFormInTheWorkspaceTimeZone(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	writeFields(t, c, "zulu@example.com", map[string]string{"trial_ends": `"2026-01-15T10:00:00Z"`})
	writeFields(t, c, "bare@example.com", map[string]string{"trial_ends": `"2026-01-15"`})
	writeFields(t, c, "local@example.com", map[string]string{"trial_ends": `"2026-01-15T10:00:00"`})
	writeFields(t, c, "text@example.com", map[string]string{"trial_ends": `"soon"`})
	writeFields(t, c, "unix@example.com", map[string]string{"trial_ends": `1768435200`})
	const beforeEve = `{"kind":"date","field":"custom:trial_ends","operator":"before","at":"2026-01-14T23:00:00Z"}`
	const afterEve = `{"kind":"date","field":"custom:trial_ends","operator":"after","at":"2026-01-14T23:00:00Z"}`

	utcBefore := previewAcme(t, c, beforeEve)
	utcAfter := previewAcme(t, c, afterEve)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetTimezone("Asia/Tokyo").ExecX(t.Context())
	tokyoBefore := previewAcme(t, c, beforeEve)
	tokyoAfter := previewAcme(t, c, afterEve)

	assert.Equal(t, int32(0), utcBefore)
	assert.Equal(t, int32(3), utcAfter, "zulu, bare and local: not text, not Unix seconds")
	assert.Equal(t, int32(1), tokyoBefore, "only bare: that day began at 15:00 UTC of the 14th in Tokyo")
	assert.Equal(t, int32(2), tokyoAfter, "zulu, which has its own offset, and local: 10:00 in Tokyo is 01:00 UTC")
}

// Changing a field's type to datetime makes its earlier ISO string values match the date leaves,
// and rewrites none of them.
func TestChangingATypeToDatetimeMakesEarlierISOStringsMatch(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	writeFields(t, c, "first@example.com", map[string]string{"renewal": `"soon"`})
	writeFields(t, c, "second@example.com", map[string]string{"renewal": `"2026-01-15T10:00:00Z"`})
	field := fieldByKey(t, c, "renewal")
	require.Equal(t, siteapi.SiteCustomFieldTypeString, field.Type)

	out, err := updateField(c, field.ID, &siteapi.SiteUpdateCustomFieldInput{Type: siteapi.NewOptSiteCustomFieldType(siteapi.SiteCustomFieldTypeDatetime)})

	require.NoError(t, err)
	updated, ok := out.(*siteapi.SiteCustomFieldResource)
	require.Truef(t, ok, "got %T", out)
	assert.Equal(t, siteapi.SiteCustomFieldTypeDatetime, updated.Type)
	assert.Equal(t, field.Key, updated.Key)
	count := previewAcme(t, c, `{"kind":"date","field":"custom:renewal","operator":"after","at":"2026-01-01T00:00:00Z"}`)
	assert.Equal(t, int32(1), count, "the ISO string matches, the text does not")
}

// A type change is refused while a Segment tests the field with an operator the new type does not
// take, naming it; a field used only with an operator every type takes changes.
func TestChangingATypeIsRefusedWhileASegmentUsesTheField(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	writeFields(t, c, "dated@example.com", map[string]string{"ends_on": `"2026-01-15"`, "has_it": `"2026-01-15"`})
	const dateLeaf = `{"kind":"group","combinator":"and","rules":[{"kind":"date","field":"custom:ends_on","operator":"after","at":"2026-01-01T00:00:00Z"}]}`
	const presenceLeaf = `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:has_it","operator":"notNull","value":""}]}`
	for name, def := range map[string]string{"Ends soon": dateLeaf, "Has it": presenceLeaf} {
		_, err := c.SiteSegmentsCreate(ctx, &siteapi.SiteCreateSegmentInput{Name: name, Condition: testhelper.SiteRuleGroup(t, def)},
			siteapi.SiteSegmentsCreateParams{WorkspaceId: fixtures.AcmePublicID})
		require.NoError(t, err)
	}
	toString := &siteapi.SiteUpdateCustomFieldInput{Type: siteapi.NewOptSiteCustomFieldType(siteapi.SiteCustomFieldTypeString)}

	refused, err := updateField(c, fieldByKey(t, c, "ends_on").ID, toString)
	require.NoError(t, err)
	changed, err := updateField(c, fieldByKey(t, c, "has_it").ID, toString)
	require.NoError(t, err)

	problem, ok := refused.(*siteapi.InUseProblemDetails)
	require.Truef(t, ok, "got %T", refused)
	assert.Equal(t, []string{"Ends soon"}, lo.Map(problem.UsedBy, func(u siteapi.UsedBy, _ int) string { return u.Name }))
	assert.Equal(t, siteapi.UsedByKindSegment, problem.UsedBy[0].Kind)
	assert.IsType(t, &siteapi.SiteCustomFieldResource{}, changed)
	assert.Equal(t, siteapi.SiteCustomFieldTypeDatetime, fieldByKey(t, c, "ends_on").Type, "the refused field keeps its type")
}

// A member may not update a field; an owner and an admin may. The key never changes, the name
// does, and each change is in the audit log.
func TestOnlyOwnersAndAdminsUpdateACustomField(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	admin := env.SiteActor(t, fixtures.AdminAliceEmail)
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	writeFields(t, owner, "named@example.com", map[string]string{"nickname": `"x"`})
	field := fieldByKey(t, owner, "nickname")

	denied, err := updateField(member, field.ID, &siteapi.SiteUpdateCustomFieldInput{Name: siteapi.NewOptString("Alias")})
	require.NoError(t, err)
	byAdmin, err := updateField(admin, field.ID, &siteapi.SiteUpdateCustomFieldInput{Name: siteapi.NewOptString("Alias")})
	require.NoError(t, err)
	byOwner, err := updateField(owner, field.ID, &siteapi.SiteUpdateCustomFieldInput{Name: siteapi.NewOptString("Nick")})
	require.NoError(t, err)

	assert.IsType(t, &siteapi.SiteCustomFieldsUpdateForbidden{}, denied)
	renamed, ok := byAdmin.(*siteapi.SiteCustomFieldResource)
	require.Truef(t, ok, "got %T", byAdmin)
	assert.Equal(t, "Alias", renamed.Name)
	assert.Equal(t, "nickname", renamed.Key)
	final, ok := byOwner.(*siteapi.SiteCustomFieldResource)
	require.Truef(t, ok, "got %T", byOwner)
	assert.Equal(t, "Nick", final.Name)
	updates := lo.FilterMap(env.OutboxEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) (*events.AuditEntry, bool) {
		e := ev.(*events.AuditEntry)
		return e, e.Action == "custom_field.update"
	})
	assert.Len(t, updates, 2)
}
