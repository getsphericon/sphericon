package site_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A send through a Test inbox says "captured, not delivered" wherever it is read, and a send through
// any other Integration does not.

func TestATransactionalSendThroughTheTestInboxIsMarkedCapturedAndAnotherIsNot(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexInboxID)
	kept := sendGlobexTransactional(t, env, "ada@example.com").(*externalapi.SendTransactionalEmailResponse)
	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexID)
	require.IsType(t, &externalapi.EmailsSendUnprocessableEntity{}, sendGlobexTransactional(t, env, "held@example.com"))

	out, err := jane.SiteTransactionalEmailsList(t.Context(), siteapi.SiteTransactionalEmailsListParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)

	page := out.(*siteapi.SiteTransactionalEmailsListOK)
	captured := lo.SliceToMap(page.Items, func(item siteapi.SiteTransactionalEmailResource) (int64, bool) {
		return int64(item.ID), item.Captured
	})
	assert.True(t, captured[int64(kept.ID)])
	assert.Equal(t, 1, lo.CountBy(page.Items, func(item siteapi.SiteTransactionalEmailResource) bool { return item.Captured }),
		"a message sent any other way is not captured")
	assert.Equal(t, 1, env.OutboxCount(t, events.NameEmailSent, map[string]any{"outboundMessageId": kept.ID, "captured": true}), "Activity says so too")
	assert.Equal(t, 1, env.OutboxCount(t, events.NameEmailDelivered, map[string]any{"outboundMessageId": kept.ID, "captured": true}))
}

func TestTheBroadcastReportCountsTheMessagesCapturedInATestInbox(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexInboxID)
	sent, err := jane.SiteBroadcastsSend(t.Context(), siteapi.SiteBroadcastsSendParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.BroadcastGlobexID),
	})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteBroadcastResource{}, sent)
	env.DeliverQueuedBroadcasts(t)

	got, err := jane.SiteBroadcastsGet(t.Context(), siteapi.SiteBroadcastsGetParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.BroadcastGlobexID),
	})
	require.NoError(t, err)

	stats := got.(*siteapi.SiteBroadcastResource).Stats
	assert.Equal(t, int32(1), stats.SentCount)
	assert.Equal(t, int32(1), stats.CapturedCount)
	assert.Equal(t, int32(1), stats.DeliveredCount, "captured messages are stamped delivered, so the report tells them apart")
}

func TestTheAutomationReportCountsTheMessagesCapturedInATestInbox(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexInboxID)
	contact := env.DB.Contact.GetX(t.Context(), fixtures.ContactGlobexReaderID)
	res, err := env.OutboundModule().Send(t.Context(), env.DB.Scoped(fixtures.GlobexID), outbound.Request{
		Kind:        outboundmessage.KindAutomation,
		Key:         "automation:captured:1",
		Destination: *contact.Email,
		Contact:     contact,
		Source:      eligibility.AutomationSource(fixtures.AutomationGlobexID),
		Subject:     "Welcome",
		Body:        "<p>Hi</p>",
		Ref:         outbound.Ref{AutomationID: fixtures.AutomationGlobexID},
	})
	require.NoError(t, err)
	require.Equal(t, outbound.Sent, res.Outcome)

	out, err := jane.SiteAutomationReportsGet(t.Context(), siteapi.SiteAutomationReportsGetParams{
		WorkspaceId: fixtures.GlobexPublicID, AutomationId: siteapi.EntityId(fixtures.AutomationGlobexID),
	})
	require.NoError(t, err)

	report := out.(*siteapi.SiteAutomationReport)
	assert.Equal(t, int32(1), report.Captured)
	assert.Equal(t, int32(1), report.Delivered)
}
