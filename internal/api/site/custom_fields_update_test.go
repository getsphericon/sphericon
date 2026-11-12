package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-faster/jx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// writeFields writes one Contact whose custom fields are the raw JSON values by key.
func writeFields(t *testing.T, c *siteapi.Client, email string, values map[string]string) {
	t.Helper()
	fields := siteapi.JsonObject{}
	for key, raw := range values {
		fields[key] = jx.Raw(raw)
	}
	_, err := c.SiteContactsCreate(context.Background(), &siteapi.SiteCreateContactInput{
		Email:        siteapi.NewOptNilEmailAddress(siteapi.EmailAddress(email)),
		CustomFields: siteapi.NewOptNilJsonObject(fields),
	}, siteapi.SiteContactsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
}

// fieldByKey is the Workspace's Custom field with the key, read from the catalogue.
func fieldByKey(t *testing.T, c *siteapi.Client, workspace siteapi.WorkspaceId, key string) siteapi.SiteCustomFieldResource {
	t.Helper()
	list, err := c.SiteCustomFieldsList(context.Background(), siteapi.SiteCustomFieldsListParams{WorkspaceId: workspace})
	require.NoError(t, err)
	field, found := lo.Find(list.Items, func(f siteapi.SiteCustomFieldResource) bool { return f.Key == key })
	require.Truef(t, found, "no custom field %q", key)
	return field
}

func updateField(c *siteapi.Client, workspace siteapi.WorkspaceId, id siteapi.EntityId, in *siteapi.SiteUpdateCustomFieldInput) (*siteapi.SiteCustomFieldResource, error) {
	return c.SiteCustomFieldsUpdate(context.Background(), in, siteapi.SiteCustomFieldsUpdateParams{WorkspaceId: workspace, ID: id})
}

// previewDuff counts Duff's Contacts that match the leaf.
func previewDuff(t *testing.T, c *siteapi.Client, leaf string) int32 {
	t.Helper()
	def := `{"kind":"group","combinator":"and","rules":[` + leaf + `]}`
	res, err := c.SiteSegmentsPreview(context.Background(), &siteapi.SitePreviewSegmentInput{
		Condition: siteapi.NewOptSiteConditionGroup(testhelper.SiteRuleGroup(t, def)),
	}, siteapi.SiteSegmentsPreviewParams{WorkspaceId: fixtures.DuffPublicID})
	require.NoError(t, err)
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

	assert.Equal(t, siteapi.SiteCustomFieldTypeDatetime, fieldByKey(t, c, fixtures.AcmePublicID, "d_zulu").Type)
	assert.Equal(t, siteapi.SiteCustomFieldTypeDatetime, fieldByKey(t, c, fixtures.AcmePublicID, "d_offset").Type)
	assert.Equal(t, siteapi.SiteCustomFieldTypeDatetime, fieldByKey(t, c, fixtures.AcmePublicID, "d_local").Type)
	assert.Equal(t, siteapi.SiteCustomFieldTypeDatetime, fieldByKey(t, c, fixtures.AcmePublicID, "d_bare").Type)
	assert.Equal(t, siteapi.SiteCustomFieldTypeString, fieldByKey(t, c, fixtures.AcmePublicID, "d_slash").Type)
	assert.Equal(t, siteapi.SiteCustomFieldTypeString, fieldByKey(t, c, fixtures.AcmePublicID, "d_text").Type)
	assert.Equal(t, siteapi.SiteCustomFieldTypeNumber, fieldByKey(t, c, fixtures.AcmePublicID, "d_number").Type)
}

// A date leaf matches a Contact holding each ISO form, an offset-less one judged in the Workspace
// time zone, and not text or Unix seconds in the field. Against the bound 2026-01-14T23:00:00Z
// (the evening before 2026-01-15), "2026-01-15" is after it in UTC and before it in Tokyo, where
// that day began at 15:00 UTC of the 14th; "2026-01-15T10:00:00" is after it in both. Duff's
// trial_ends holds zulu, bare and local ISO forms, text and Unix seconds, one Contact each.
func TestADateLeafReadsEachISOFormInTheWorkspaceTimeZone(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DuffOwnerDanEmail)
	const beforeEve = `{"kind":"date","field":"custom:trial_ends","operator":"before","at":"2026-01-14T23:00:00Z"}`
	const afterEve = `{"kind":"date","field":"custom:trial_ends","operator":"after","at":"2026-01-14T23:00:00Z"}`

	utcBefore := previewDuff(t, c, beforeEve)
	utcAfter := previewDuff(t, c, afterEve)
	env.DB.Workspace.UpdateOneID(fixtures.DuffID).SetTimezone("Asia/Tokyo").ExecX(t.Context())
	tokyoBefore := previewDuff(t, c, beforeEve)
	tokyoAfter := previewDuff(t, c, afterEve)

	assert.Equal(t, int32(0), utcBefore)
	assert.Equal(t, int32(3), utcAfter, "zulu, bare and local: not text, not Unix seconds")
	assert.Equal(t, int32(1), tokyoBefore, "only bare: that day began at 15:00 UTC of the 14th in Tokyo")
	assert.Equal(t, int32(2), tokyoAfter, "zulu, which has its own offset, and local: 10:00 in Tokyo is 01:00 UTC")
}

// Changing a field's type to datetime makes its earlier ISO string values match the date leaves,
// and rewrites none of them. Duff's string field renewal holds "soon" and an ISO string.
func TestChangingATypeToDatetimeMakesEarlierISOStringsMatch(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DuffOwnerDanEmail)
	field := fieldByKey(t, c, fixtures.DuffPublicID, "renewal")
	require.Equal(t, siteapi.SiteCustomFieldTypeString, field.Type)

	updated, err := updateField(c, fixtures.DuffPublicID, field.ID, &siteapi.SiteUpdateCustomFieldInput{Revision: field.Revision, Type: siteapi.NewOptSiteCustomFieldType(siteapi.SiteCustomFieldTypeDatetime)})

	require.NoError(t, err)
	assert.Equal(t, siteapi.SiteCustomFieldTypeDatetime, updated.Type)
	assert.Equal(t, field.Key, updated.Key)
	count := previewDuff(t, c, `{"kind":"date","field":"custom:renewal","operator":"after","at":"2026-01-01T00:00:00Z"}`)
	assert.Equal(t, int32(1), count, "the ISO string matches, the text does not")
}

// A type change is refused while a Segment tests the field with an operator the new type does not
// take, naming it; a field used only with an operator every type takes changes. Duff's Segment
// "Ends soon" tests the datetime ends_on with a date operator, "Has it" only that has_it is there.
func TestChangingATypeIsRefusedWhileASegmentUsesTheField(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DuffOwnerDanEmail)
	toString := func(key string) (siteapi.EntityId, *siteapi.SiteUpdateCustomFieldInput) {
		field := fieldByKey(t, c, fixtures.DuffPublicID, key)
		return field.ID, &siteapi.SiteUpdateCustomFieldInput{Revision: field.Revision, Type: siteapi.NewOptSiteCustomFieldType(siteapi.SiteCustomFieldTypeString)}
	}

	endsOn, in := toString("ends_on")
	_, err := updateField(c, fixtures.DuffPublicID, endsOn, in)
	problem := testhelper.SiteRefusal(t, err, http.StatusConflict)
	hasIt, in := toString("has_it")
	_, err = updateField(c, fixtures.DuffPublicID, hasIt, in)
	require.NoError(t, err)

	assert.Equal(t, []string{"Ends soon"}, lo.Map(problem.UsedBy, func(u siteapi.UsedBy, _ int) string { return u.Name }))
	assert.Equal(t, siteapi.UsedByKindSegment, problem.UsedBy[0].Kind)
	assert.Equal(t, siteapi.SiteCustomFieldTypeDatetime, fieldByKey(t, c, fixtures.DuffPublicID, "ends_on").Type, "the refused field keeps its type")
}

// A member may not update a field; an owner and an admin may. The key never changes, the name
// does, and each change is in the audit log. Acme's field is plan.
func TestOnlyOwnersAndAdminsUpdateACustomField(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	admin := env.SiteActor(t, fixtures.AdminAliceEmail)
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	field := fieldByKey(t, owner, fixtures.AcmePublicID, "plan")

	_, err := updateField(member, fixtures.AcmePublicID, field.ID, &siteapi.SiteUpdateCustomFieldInput{Revision: field.Revision, Name: siteapi.NewOptName("Alias")})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)
	renamed, err := updateField(admin, fixtures.AcmePublicID, field.ID, &siteapi.SiteUpdateCustomFieldInput{Revision: field.Revision, Name: siteapi.NewOptName("Alias")})
	require.NoError(t, err)
	final, err := updateField(owner, fixtures.AcmePublicID, field.ID, &siteapi.SiteUpdateCustomFieldInput{Revision: renamed.Revision, Name: siteapi.NewOptName("Nick")})
	require.NoError(t, err)

	assert.Equal(t, "Alias", renamed.Name)
	assert.Equal(t, "plan", renamed.Key)
	assert.Equal(t, "Nick", final.Name)
	updates := lo.FilterMap(env.QueuedEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) (*events.AuditEntry, bool) {
		e := ev.(*events.AuditEntry)
		return e, e.Action == "custom_field.update"
	})
	assert.Len(t, updates, 2)
}

// One Custom field reads back as the list shows it, with the revision its editor opens with; another
// Workspace's is a 404.
func TestSiteCustomFieldGetReadsOneField(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	field := fieldByKey(t, c, fixtures.AcmePublicID, "plan")

	got, err := c.SiteCustomFieldsGet(context.Background(), siteapi.SiteCustomFieldsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: field.ID})
	require.NoError(t, err)
	assert.Equal(t, field, *got)

	_, err = c.SiteCustomFieldsGet(context.Background(), siteapi.SiteCustomFieldsGetParams{WorkspaceId: fixtures.GlobexPublicID, ID: field.ID})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}
