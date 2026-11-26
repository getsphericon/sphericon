package site_test

import (
	"context"
	"net/http"
	"testing"
	"time"

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
		Status:       siteapi.NewOptEnrollmentStatus(siteapi.EnrollmentStatusFailed),
	})
	require.NoError(t, err)
	failedRun, found := lo.Find(listed.Items, func(e siteapi.EnrollmentResource) bool {
		return e.ID == siteapi.EntityId(fixtures.AutomationRunHTTPFailedID)
	})
	require.True(t, found)
	f, ok := failedRun.Failure.Get()
	require.True(t, ok)
	assert.Equal(t, siteapi.EnrollmentFailureKindHTTPStatus, f.Kind)
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

// The console reads an Automation's skipped entries, filtered by Contact (ADR 0127).
func TestSiteSkippedEntriesOfAContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)
	ctx := context.Background()

	listed, err := c.SiteSkippedEntriesList(ctx, siteapi.SiteSkippedEntriesListParams{
		WorkspaceId:  fixtures.AcmePublicID,
		AutomationId: siteapi.EntityId(fixtures.AutomationWelcomeSeriesID),
		ContactId:    siteapi.NewOptEntityId(siteapi.EntityId(fixtures.ContactEmmaID)),
	})
	require.NoError(t, err)
	require.Len(t, listed.Items, 1)
	assert.Equal(t, siteapi.EntityId(fixtures.SkippedEntryWelcomeEmmaID), listed.Items[0].ID)
	assert.Equal(t, siteapi.SkippedEntryReasonActiveEnrollment, listed.Items[0].Reason)

	_, err = c.SiteSkippedEntriesList(ctx, siteapi.SiteSkippedEntriesListParams{
		WorkspaceId: fixtures.AcmePublicID, AutomationId: siteapi.EntityId(fixtures.AutomationGlobexID),
	})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}

// An Enrollment of a date Trigger carries the occurrence it was made for (ADR 0126): its
// anniversary year (fixture Birthday greeting: Alice's for 2026).
func TestSiteEnrollmentsShowTheAnniversaryYearOfADateTrigger(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	dated, err := c.SiteEnrollmentsGet(t.Context(), siteapi.SiteEnrollmentsGetParams{
		WorkspaceId:  fixtures.AcmePublicID,
		AutomationId: siteapi.EntityId(fixtures.AutomationBirthdayGreetingID),
		EnrollmentId: siteapi.EntityId(fixtures.AutomationRunBirthdayGreetingAliceID),
	})
	require.NoError(t, err)

	occurrence, ok := dated.Occurrence.Get()
	require.True(t, ok)
	assert.JSONEq(t, `{"recurrence":"yearly","year":2026}`, testhelper.JSONOf(t, &occurrence))
}

// An Enrollment of a once date Trigger carries the once date's one occurrence, and no year.
func TestSiteEnrollmentsShowTheOnceOccurrence(t *testing.T) {
	clock := testhelper.NewClock(time.Date(2027, time.March, 19, 12, 0, 0, 0, time.UTC))
	env := testhelper.Setup(t, testhelper.WithClock(clock.Now))
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	id := env.OnceDateEnrollment(t, clock)

	listed, err := c.SiteEnrollmentsList(t.Context(), siteapi.SiteEnrollmentsListParams{
		WorkspaceId: fixtures.AcmePublicID, AutomationId: siteapi.EntityId(id),
	})
	require.NoError(t, err)

	require.Len(t, listed.Items, 1)
	occurrence, ok := listed.Items[0].Occurrence.Get()
	require.True(t, ok)
	assert.JSONEq(t, `{"recurrence":"once"}`, testhelper.JSONOf(t, &occurrence))
}
