package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

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

	_, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name:      "retired",
		Content:   testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{}),
		SegmentId: siteapi.NewOptNilEntityId(idStr(fixtures.SegmentArchivedID)),
	}, workspaceID)
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	_, err = c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name:          "retired",
		Content:       testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{}),
		IntegrationId: siteapi.NewOptNilEntityId(idStr(fixtures.IntegrationAcmeArchivedID)),
	}, workspaceID)
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	after := env.DB.Broadcast.Query().CountX(ctx)
	assert.Equal(t, before, after, "a refused create stores nothing")
}

func TestSiteBroadcastsUpdateRefusesAnArchivedSegmentOrIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastDraftID)}
	before := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)

	_, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Revision: siteapi.Revision(env.BroadcastRevision(ctx, t, int64(params.ID))),
		SegmentId: siteapi.NewOptNilEntityId(idStr(fixtures.SegmentArchivedID)),
	}, params)
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	_, err = c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Revision: siteapi.Revision(env.BroadcastRevision(ctx, t, int64(params.ID))),
		IntegrationId: siteapi.NewOptNilEntityId(idStr(fixtures.IntegrationAcmeArchivedID)),
	}, params)
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	after := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)
	assert.Equal(t, before.SegmentID, after.SegmentID, "a refused update changes nothing")
	assert.Equal(t, before.IntegrationID, after.IntegrationID)
}
