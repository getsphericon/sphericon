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

func TestSiteEnrollmentsShowWhyAnEnrollmentFailed(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	list, err := c.SiteEnrollmentsList(ctx, siteapi.SiteEnrollmentsListParams{
		WorkspaceId:  fixtures.AcmePublicID,
		AutomationId: siteapi.EntityId(fixtures.AutomationNotifySlackID),
		Status:       siteapi.NewOptSiteEnrollmentStatus(siteapi.SiteEnrollmentStatusFailed),
	})
	require.NoError(t, err)
	listed, ok := list.(*siteapi.SiteEnrollmentsListOK)
	require.Truef(t, ok, "got %T", list)
	var found bool
	for _, e := range listed.Items {
		if e.ID != siteapi.EntityId(fixtures.AutomationRunHTTPFailedID) {
			continue
		}
		found = true
		f, ok := e.Failure.Get()
		require.True(t, ok)
		assert.Equal(t, siteapi.SiteEnrollmentFailureKindHTTPStatus, f.Kind)
		assert.EqualValues(t, 503, f.Status.Or(0))
		assert.Equal(t, "upstream unavailable, try again later", f.Excerpt.Or(""))
	}
	assert.True(t, found)

	got, err := c.SiteEnrollmentsGet(ctx, siteapi.SiteEnrollmentsGetParams{
		WorkspaceId:  fixtures.AcmePublicID,
		AutomationId: siteapi.EntityId(fixtures.AutomationNotifySlackID),
		EnrollmentId: siteapi.EntityId(fixtures.AutomationRunHTTPFailedID),
	})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteEnrollmentResource{}, got)

	foreign, err := c.SiteEnrollmentsList(ctx, siteapi.SiteEnrollmentsListParams{
		WorkspaceId: fixtures.AcmePublicID, AutomationId: siteapi.EntityId(fixtures.AutomationGlobexID),
	})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteEnrollmentsListNotFound{}, foreign)
}
