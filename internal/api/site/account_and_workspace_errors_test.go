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

}
