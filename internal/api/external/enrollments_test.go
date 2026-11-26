package external_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A failed Enrollment of the NotifySlack automation carries its closed error kind, the
// receiver's status and the start of its answer.
func TestExternalEnrollmentsReadAFailedEnrollment(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, apitokens.ScopeEnrollmentsRead)

	listed, err := c.EnrollmentsList(ctx, externalapi.EnrollmentsListParams{
		AutomationId: entityIDString(fixtures.AutomationNotifySlackID),
		Status:       externalapi.NewOptEnrollmentStatus(externalapi.EnrollmentStatusFailed),
	})
	require.NoError(t, err)
	failed, found := lo.Find(listed.Items, func(e externalapi.EnrollmentResource) bool {
		return e.ID == entityIDString(fixtures.AutomationRunHTTPFailedID)
	})
	require.True(t, found, "the failed Enrollment is listed")
	assert.Equal(t, externalapi.EnrollmentStatusFailed, failed.Status)
	f, ok := failed.Failure.Get()
	require.True(t, ok)
	assert.Equal(t, externalapi.EnrollmentFailureKindHTTPStatus, f.Kind)
	assert.EqualValues(t, 503, f.Status.Or(0))
	assert.Equal(t, "upstream unavailable, try again later", f.Excerpt.Or(""))

	one, err := c.EnrollmentsGet(ctx, externalapi.EnrollmentsGetParams{
		AutomationId: entityIDString(fixtures.AutomationNotifySlackID),
		EnrollmentId: entityIDString(fixtures.AutomationRunHTTPFailedID),
	})
	require.NoError(t, err)
	assert.Equal(t, failed.Failure, one.Failure)
}

func TestExternalEnrollmentsAreFilteredByStatusAndScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, apitokens.ScopeEnrollmentsRead)

	completed, err := c.EnrollmentsList(ctx, externalapi.EnrollmentsListParams{
		AutomationId: entityIDString(fixtures.AutomationWelcomeSeriesID),
		Status:       externalapi.NewOptEnrollmentStatus(externalapi.EnrollmentStatusCompleted),
	})
	require.NoError(t, err)
	items := completed.Items
	require.NotEmpty(t, items)
	for _, e := range items {
		assert.Equal(t, externalapi.EnrollmentStatusCompleted, e.Status)
		assert.Equal(t, entityIDString(fixtures.AutomationWelcomeSeriesID), e.AutomationId)
	}

	// Another Workspace's Automation, and an Enrollment of a different Automation, are not found.
	_, err = c.EnrollmentsList(ctx, externalapi.EnrollmentsListParams{AutomationId: entityIDString(fixtures.AutomationGlobexID)})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
	_, err = c.EnrollmentsGet(ctx, externalapi.EnrollmentsGetParams{
		AutomationId: entityIDString(fixtures.AutomationWelcomeSeriesID),
		EnrollmentId: entityIDString(fixtures.AutomationRunHTTPFailedID),
	})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
}

func TestExternalEnrollmentsNeedTheirOwnScope(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	denied := env.ExternalScoped(t, apitokens.ScopeAutomationsRead)

	_, err := denied.EnrollmentsList(ctx, externalapi.EnrollmentsListParams{AutomationId: entityIDString(fixtures.AutomationNotifySlackID)})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	_, err = denied.EnrollmentsGet(ctx, externalapi.EnrollmentsGetParams{
		AutomationId: entityIDString(fixtures.AutomationNotifySlackID),
		EnrollmentId: entityIDString(fixtures.AutomationRunHTTPFailedID),
	})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
}

// An Enrollment of a date Trigger carries the occurrence it was made for (ADR 0126): its
// anniversary year (fixture Birthday greeting: Alice's for 2026). One an Event began carries none.
func TestExternalEnrollmentsShowTheAnniversaryYearOfADateTrigger(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.ExternalScoped(t, apitokens.ScopeEnrollmentsRead)

	dated, err := c.EnrollmentsGet(ctx, externalapi.EnrollmentsGetParams{
		AutomationId: entityIDString(fixtures.AutomationBirthdayGreetingID),
		EnrollmentId: entityIDString(fixtures.AutomationRunBirthdayGreetingAliceID),
	})
	require.NoError(t, err)
	evented, err := c.EnrollmentsGet(ctx, externalapi.EnrollmentsGetParams{
		AutomationId: entityIDString(fixtures.AutomationWelcomeSeriesID),
		EnrollmentId: entityIDString(fixtures.AutomationRunWelcomeCompletedID),
	})
	require.NoError(t, err)

	occurrence, ok := dated.Occurrence.Get()
	require.True(t, ok)
	assert.JSONEq(t, `{"recurrence":"yearly","year":2026}`, testhelper.JSONOf(t, &occurrence))
	assert.False(t, evented.Occurrence.IsSet())
}

// An Enrollment of a once date Trigger carries the once date's one occurrence, and no year.
func TestExternalEnrollmentsShowTheOnceOccurrence(t *testing.T) {
	clock := testhelper.NewClock(time.Date(2027, time.March, 19, 12, 0, 0, 0, time.UTC))
	env := testhelper.Setup(t, testhelper.WithClock(clock.Now))
	c := env.ExternalScoped(t, apitokens.ScopeEnrollmentsRead)
	id := env.OnceDateEnrollment(t, clock)

	listed, err := c.EnrollmentsList(t.Context(), externalapi.EnrollmentsListParams{AutomationId: entityIDString(id)})
	require.NoError(t, err)

	require.Len(t, listed.Items, 1)
	occurrence, ok := listed.Items[0].Occurrence.Get()
	require.True(t, ok)
	assert.JSONEq(t, `{"recurrence":"once"}`, testhelper.JSONOf(t, &occurrence))
}
