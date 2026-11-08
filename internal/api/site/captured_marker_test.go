package site_test

import (
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A send through a Test inbox says "captured, not delivered" wherever it is read, and a send through
// any other Integration does not.

func TestATransactionalSendThroughTheTestInboxIsMarkedCapturedAndAnotherIsNot(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexInboxID)
	kept, err := sendGlobexTransactional(t, env, "ada@example.com")
	require.NoError(t, err)
	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexID)
	_, err = sendGlobexTransactional(t, env, "held@example.com")
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)

	page, err := jane.SiteTransactionalEmailsList(t.Context(), siteapi.SiteTransactionalEmailsListParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)

	captured := lo.SliceToMap(page.Items, func(item siteapi.SiteTransactionalEmailResource) (int64, bool) {
		return int64(item.ID), item.Captured
	})
	assert.True(t, captured[int64(kept.ID)])
	assert.Equal(t, 1, lo.CountBy(page.Items, func(item siteapi.SiteTransactionalEmailResource) bool { return item.Captured }),
		"a message sent any other way is not captured")
	assert.Equal(t, 1, env.QueuedCount(t, events.NameEmailSent, map[string]any{"outboundMessageId": kept.ID, "captured": true}), "Activity says so too")
	assert.Equal(t, 1, env.QueuedCount(t, events.NameEmailDelivered, map[string]any{"outboundMessageId": kept.ID, "captured": true}))
}

func TestTheBroadcastReportCountsTheMessagesCapturedInATestInbox(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexInboxID)
	_, err := jane.SiteBroadcastsSend(t.Context(), siteapi.SiteBroadcastsSendParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.BroadcastGlobexID),
	})
	require.NoError(t, err)

	env.DeliverQueuedBroadcasts(t)
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})

	got, err := jane.SiteBroadcastsGet(t.Context(), siteapi.SiteBroadcastsGetParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.BroadcastGlobexID),
	})
	require.NoError(t, err)

	stats := got.Stats
	assert.Equal(t, int32(1), stats.SentCount)
	assert.Equal(t, int32(1), stats.CapturedCount.Value)
	assert.Equal(t, int32(1), stats.DeliveredCount.Value, "captured messages are stamped delivered, so the report tells them apart")
}

func TestTheAutomationReportCountsTheMessagesCapturedInATestInbox(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)

	out, err := jane.SiteAutomationReportsGet(t.Context(), siteapi.SiteAutomationReportsGetParams{
		WorkspaceId: fixtures.GlobexPublicID, AutomationId: siteapi.EntityId(fixtures.AutomationGlobexID),
	})
	require.NoError(t, err)

	report := out
	assert.Equal(t, int32(1), report.Captured)
	assert.Equal(t, int32(1), report.Delivered)
}
