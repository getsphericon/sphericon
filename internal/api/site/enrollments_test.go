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
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteEnrollmentsShowWhyAnEnrollmentFailed(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	listed, err := c.SiteEnrollmentsList(ctx, siteapi.SiteEnrollmentsListParams{
		WorkspaceId:  fixtures.AcmePublicID,
		AutomationId: siteapi.EntityId(fixtures.AutomationNotifySlackID),
		Status:       siteapi.NewOptSiteEnrollmentStatus(siteapi.SiteEnrollmentStatusFailed),
	})
	require.NoError(t, err)
	failedRun, found := lo.Find(listed.Items, func(e siteapi.SiteEnrollmentResource) bool {
		return e.ID == siteapi.EntityId(fixtures.AutomationRunHTTPFailedID)
	})
	require.True(t, found)
	f, ok := failedRun.Failure.Get()
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteEnrollmentFailureKindHTTPStatus, f.Kind)
	assert.EqualValues(t, 503, f.Status.Or(0))
	assert.Equal(t, "upstream unavailable, try again later", f.Excerpt.Or(""))

	_, err = c.SiteEnrollmentsGet(ctx, siteapi.SiteEnrollmentsGetParams{
		WorkspaceId:  fixtures.AcmePublicID,
		AutomationId: siteapi.EntityId(fixtures.AutomationNotifySlackID),
		EnrollmentId: siteapi.EntityId(fixtures.AutomationRunHTTPFailedID),
	})
	require.NoError(t, err)

	_, err = c.SiteEnrollmentsList(ctx, siteapi.SiteEnrollmentsListParams{
		WorkspaceId: fixtures.AcmePublicID, AutomationId: siteapi.EntityId(fixtures.AutomationGlobexID),
	})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}
