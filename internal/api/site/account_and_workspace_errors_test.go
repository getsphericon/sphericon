package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/event"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteContactsCustomFieldsRoundTrip(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	created, err := c.SiteContactsCreate(ctx, &siteapi.SiteCreateContactInput{
		Email:        siteapi.NewOptNilEmailAddress("cf@example.com"),
		CustomFields: siteapi.NewOptNilSiteCreateContactInputCustomFields(siteapi.SiteCreateContactInputCustomFields{"plan": []byte(`"gold"`)}),
	}, siteapi.SiteContactsCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteContactResource)
	require.Truef(t, ok, "got %T", created)
	assert.JSONEq(t, `"gold"`, string(res.CustomFields.Value["plan"]))

	updated, err := c.SiteContactsUpdate(ctx, &siteapi.SiteUpdateContactInput{
		CustomFields: siteapi.NewOptNilSiteUpdateContactInputCustomFields(siteapi.SiteUpdateContactInputCustomFields{"plan": []byte(`"platinum"`)}),
	}, siteapi.SiteContactsUpdateParams{Slug: fixtures.AcmeSlug, ID: res.ID})
	require.NoError(t, err)
	upd, ok := updated.(*siteapi.SiteContactResource)
	require.Truef(t, ok, "got %T", updated)
	assert.JSONEq(t, `"platinum"`, string(upd.CustomFields.Value["plan"]))
}

func TestSiteEventsListFiltersByContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	all, err := c.SiteEventsList(ctx, siteapi.SiteEventsListParams{Slug: fixtures.AcmeSlug, PageSize: siteapi.NewOptInt32(100)})
	require.NoError(t, err)
	allPage := all.(*siteapi.SiteEventsListOK)

	res, err := c.SiteEventsList(ctx, siteapi.SiteEventsListParams{
		Slug: fixtures.AcmeSlug, ContactId: siteapi.NewOptEntityId(idStr(fixtures.ContactAliceID)),
	})
	require.NoError(t, err)
	page, ok := res.(*siteapi.SiteEventsListOK)
	require.Truef(t, ok, "got %T", res)
	assert.Less(t, page.TotalItems, allPage.TotalItems, "the contact filter narrows the list")
	want, err := env.DB.Event.Query().Where(event.WorkspaceID(fixtures.AcmeID), event.ContactID(fixtures.ContactAliceID)).Count(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, want, page.TotalItems, "exactly alice's events")

	// An id that does not fit an int64 is ignored rather than failing the request.
	loose, err := c.SiteEventsList(ctx, siteapi.SiteEventsListParams{
		Slug: fixtures.AcmeSlug, ContactId: siteapi.NewOptEntityId(overflowID), PageSize: siteapi.NewOptInt32(100),
	})
	require.NoError(t, err)
	assert.Equal(t, allPage.TotalItems, loose.(*siteapi.SiteEventsListOK).TotalItems)
}

func TestSiteWorkspaceScopedListsRequireMembership(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	tx, err := c.SiteTransactionalEmailsList(ctx, siteapi.SiteTransactionalEmailsListParams{Slug: fixtures.GlobexSlug})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTransactionalEmailsListNotFound{}, tx)

	prev, err := c.SiteSegmentsPreview(ctx, &siteapi.SitePreviewSegmentInput{}, siteapi.SiteSegmentsPreviewParams{Slug: fixtures.GlobexSlug})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsPreviewNotFound{}, prev)
}

func TestSiteWorkspacesUpdateSetsPostalAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	res, err := c.SiteWorkspacesUpdate(ctx, &siteapi.SiteUpdateWorkspaceInput{
		Name: "Acme Inc", PostalAddress: siteapi.NewOptString("  1 Main St  "),
	}, siteapi.SiteWorkspacesUpdateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteWorkspaceResource{}, res)
	ws, err := env.DB.Workspace.Get(ctx, fixtures.AcmeID)
	require.NoError(t, err)
	assert.Equal(t, "Acme Inc", ws.Name)
	assert.Equal(t, "1 Main St", ws.PostalAddress)
}

func TestSiteUserUpdateMeRequiresCurrentPasswordForNewOne(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	res, err := c.SiteUserUpdateMe(ctx, &siteapi.SiteUpdateMeInput{NewPassword: siteapi.NewOptString("brandnew-pass-1")})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteUserUpdateMeUnprocessableEntity{}, res)
	assert.Equal(t, 200, loginStatus(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword), "the old password still works")
}
