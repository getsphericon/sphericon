package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An archived Segment or Integration leaves new work: a Broadcast cannot be created with one.
func TestSiteBroadcastsCreateRefusesAnArchivedSegmentOrIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID}
	before := env.DB.Broadcast.Query().CountX(ctx)

	got, err := c.SiteBroadcastsCreate(ctx, &siteapi.SiteCreateBroadcastInput{
		Name: "retired", SegmentId: siteapi.NewOptNilEntityId(idStr(fixtures.SegmentArchivedID)),
	}, workspaceID)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsCreateUnprocessableEntity{}, got)

	got, err = c.SiteBroadcastsCreate(ctx, &siteapi.SiteCreateBroadcastInput{
		Name: "retired", IntegrationId: siteapi.NewOptNilEntityId(idStr(fixtures.IntegrationAcmeArchivedID)),
	}, workspaceID)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsCreateUnprocessableEntity{}, got)

	after := env.DB.Broadcast.Query().CountX(ctx)
	assert.Equal(t, before, after, "a refused create stores nothing")
}

func TestSiteBroadcastsUpdateRefusesAnArchivedSegmentOrIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastDraftID)}
	before := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)

	got, err := c.SiteBroadcastsUpdate(ctx, &siteapi.SiteUpdateBroadcastInput{
		SegmentId: siteapi.NewOptNilEntityId(idStr(fixtures.SegmentArchivedID)),
	}, params)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsUpdateUnprocessableEntity{}, got)

	got, err = c.SiteBroadcastsUpdate(ctx, &siteapi.SiteUpdateBroadcastInput{
		IntegrationId: siteapi.NewOptNilEntityId(idStr(fixtures.IntegrationAcmeArchivedID)),
	}, params)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsUpdateUnprocessableEntity{}, got)

	after := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)
	assert.Equal(t, before.SegmentID, after.SegmentID, "a refused update changes nothing")
	assert.Equal(t, before.IntegrationID, after.IntegrationID)
}
