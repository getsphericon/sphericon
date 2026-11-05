package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/narrow"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteSegmentContactsListReturnsOnlyMatchingContacts(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	res, err := c.SiteSegmentContactsList(ctx, siteapi.SiteSegmentContactsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		SegmentId:   siteapi.EntityId(fixtures.SegmentProPlanID),
		PageSize:    siteapi.NewOptInt32(100),
	})
	require.NoError(t, err)
	emails := lo.Map(res.Items, func(item siteapi.SiteContactResource, _ int) string {
		return string(item.Email.Value)
	})
	assert.Contains(t, emails, fixtures.ContactLiamEmail)
	assert.NotContains(t, emails, fixtures.ContactNoahEmail, "a contact on another plan is outside the segment")
	assert.Equal(t, narrow.Int32(len(res.Items)), res.TotalItems)
}

func TestSiteSegmentContactsListUnknownSegmentIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteSegmentContactsList(context.Background(), siteapi.SiteSegmentContactsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		SegmentId:   siteapi.EntityId(999999),
	})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}
