package site_test

import (
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteSegmentContactsListReturnsOnlyMatchingContacts(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()

	res, err := c.SiteSegmentContactsList(ctx, siteapi.SiteSegmentContactsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		SegmentId:   siteapi.EntityId(fixtures.SegmentProPlanID),
		Limit:       siteapi.NewOptInt32(100),
	})
	require.NoError(t, err)
	emails := lo.Map(res.Items, func(item siteapi.SiteContactResource, _ int) string {
		return string(item.Email.Value)
	})
	assert.Contains(t, emails, fixtures.ContactLiamEmail)
	assert.NotContains(t, emails, fixtures.ContactNoahEmail, "a contact on another plan is outside the segment")
	assert.False(t, res.NextCursor.Set, "one page holds them all")
}

// The list is read by a cursor: a page of one hands out the cursor of the next, which reads on
// from the Contact after it.
func TestSiteSegmentContactsListPagesByCursor(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	params := siteapi.SiteSegmentContactsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		SegmentId:   siteapi.EntityId(fixtures.SegmentProPlanID),
		Limit:       siteapi.NewOptInt32(100),
	}
	all, err := c.SiteSegmentContactsList(t.Context(), params)
	require.NoError(t, err)
	require.Greater(t, len(all.Items), 1)
	params.Limit = siteapi.NewOptInt32(1)

	first, err := c.SiteSegmentContactsList(t.Context(), params)
	require.NoError(t, err)
	params.Cursor = siteapi.NewOptString(first.NextCursor.Value)
	second, err := c.SiteSegmentContactsList(t.Context(), params)
	require.NoError(t, err)

	assert.Equal(t, []siteapi.SiteContactResource{all.Items[0]}, first.Items)
	assert.Equal(t, []siteapi.SiteContactResource{all.Items[1]}, second.Items)
}

// A cursor no page handed out is refused.
func TestSiteSegmentContactsListRefusesABadCursor(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteSegmentContactsList(t.Context(), siteapi.SiteSegmentContactsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		SegmentId:   siteapi.EntityId(fixtures.SegmentProPlanID),
		Cursor:      siteapi.NewOptString("not-a-cursor"),
	})

	testhelper.SiteRefusal(t, err, http.StatusBadRequest)
}

func TestSiteSegmentContactsListUnknownSegmentIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteSegmentContactsList(t.Context(), siteapi.SiteSegmentContactsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		SegmentId:   siteapi.EntityId(999999),
	})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}
