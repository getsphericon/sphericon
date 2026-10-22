package site_test

import (
	"context"
	"testing"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteSegmentContactsListReturnsOnlyMatchingContacts(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	_, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("in-segment@test.dev").
		SetCustomFields(map[string]any{"plan": "listed-pro"}).Save(ctx)
	require.NoError(t, err)
	_, err = env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("outside@test.dev").
		SetCustomFields(map[string]any{"plan": "free"}).Save(ctx)
	require.NoError(t, err)
	seg, err := env.DB.Segment.Create().SetWorkspaceID(fixtures.AcmeID).SetName("Listed pro").
		SetDefinition(`{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:plan","operator":"eq","value":"listed-pro"}]}`).
		Save(ctx)
	require.NoError(t, err)

	out, err := c.SiteSegmentContactsList(ctx, siteapi.SiteSegmentContactsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		SegmentId:   siteapi.EntityId(seg.ID),
	})
	require.NoError(t, err)
	res, ok := out.(*siteapi.SiteSegmentContactsListOK)
	require.Truef(t, ok, "got %T", out)
	require.Len(t, res.Items, 1)
	assert.Equal(t, siteapi.EmailAddress("in-segment@test.dev"), res.Items[0].Email.Value)
	assert.Equal(t, int32(1), res.TotalItems)
}

func TestSiteSegmentContactsListUnknownSegmentIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	out, err := c.SiteSegmentContactsList(context.Background(), siteapi.SiteSegmentContactsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		SegmentId:   siteapi.EntityId(999999),
	})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentContactsListNotFound{}, out)
}
