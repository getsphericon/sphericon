package site_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/capturedmessage"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func inboxInput() siteapi.IntegrationConfigInput {
	return siteapi.IntegrationConfigInput{
		OneOf: siteapi.NewInboxConfigInputIntegrationConfigInputSum(siteapi.InboxConfigInput{
			Kind: siteapi.InboxConfigInputKindInbox,
		}),
	}
}

func createInbox(t *testing.T, c *siteapi.Client, workspace siteapi.WorkspaceId) (*siteapi.SiteIntegrationResource, error) {
	t.Helper()
	return c.SiteIntegrationsCreate(context.Background(), &siteapi.CreateIntegrationInput{
		Name: "Test inbox", Config: inboxInput(),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: workspace})
}

// makeDefault picks an Integration as its Workspace's default, the way any Integration is chosen.
func makeDefault(t *testing.T, c *siteapi.Client, workspace siteapi.WorkspaceId, id int64) {
	t.Helper()
	_, err := c.SiteIntegrationsUpdate(context.Background(), &siteapi.UpdateIntegrationInput{
		IsDefault: siteapi.NewOptBool(true),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: workspace, ID: idStr(id)})
	require.NoError(t, err)
}

func sendGlobexTransactional(t *testing.T, env *testhelper.TestEnv, to string) (*externalapi.SendTransactionalEmailResponse, error) {
	t.Helper()
	c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.GlobexID, "emails:send"))
	return c.EmailsSend(context.Background(), &externalapi.SendTransactionalEmailInput{
		TemplateId: externalapi.EntityId(fixtures.TemplateGlobexID), Destination: externalapi.EmailAddress(to),
	}, externalapi.EmailsSendParams{})
}

func TestSiteTestInboxIsCreatedWithoutCredentialsAndIsNotTheDefault(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	created, err := createInbox(t, c, fixtures.AcmePublicID)

	require.NoError(t, err)
	assert.Equal(t, siteapi.SiteIntegrationProviderInbox, created.Provider)
	assert.Equal(t, siteapi.SiteIntegrationChannelEmail, created.Channel)
	assert.False(t, created.IsDefault, "a Test inbox never becomes the default by itself")
	kind, ok := created.Config.OneOf.GetSiteInboxConfig()
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteInboxConfigKindInbox, kind.Kind)
	defaults := env.DB.Integration.Query().
		Where(integration.WorkspaceID(fixtures.AcmeID), integration.IsDefault(true)).CountX(t.Context())
	assert.Equal(t, 1, defaults, "the existing default is untouched")
	env.PerformEnqueuedJobs(t, integrations.RefreshQuotaArgs{})
	reread, err := c.SiteIntegrationsGet(t.Context(), siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID})
	require.NoError(t, err)
	limit := reread.ProviderLimit.PerDay
	assert.Equal(t, int32(200), limit.Limit.Value, "it reports its own ceiling")
	assert.Equal(t, siteapi.NewNilSiteProviderLimitSource(siteapi.SiteProviderLimitSourceProvider), limit.Source)
}

func TestSiteTestInboxCanBeChosenAsTheDefaultLikeAnyIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)

	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexInboxID)

	row := env.DB.Integration.GetX(t.Context(), fixtures.IntegrationGlobexInboxID)
	assert.True(t, row.IsDefault)
}

func TestSiteSecondTestInboxIsRefusedNamingTheRule(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)

	// Globex keeps a Test inbox already.
	_, err := createInbox(t, jane, fixtures.GlobexPublicID)

	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Contains(t, refused.Detail.Value, "at most one Test inbox")
}

func TestSiteDeletingATestInboxFreesThePlaceForAnother(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	err := jane.SiteIntegrationsDelete(t.Context(),
		siteapi.SiteIntegrationsDeleteParams{WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.IntegrationGlobexInboxID)})
	require.NoError(t, err)

	_, err = createInbox(t, jane, fixtures.GlobexPublicID)
	assert.NoError(t, err, "the deleted one frees the place")
}

func TestSiteCapturedMessagesAreListedNewestFirstAndOnlyForTheOwnWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	page, err := jane.SiteCapturedMessagesList(t.Context(), siteapi.SiteCapturedMessagesListParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	empty, err := john.SiteCapturedMessagesList(t.Context(), siteapi.SiteCapturedMessagesListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	require.Len(t, page.Items, 2)
	assert.Equal(t, int32(2), page.TotalItems)
	assert.Equal(t, int64(fixtures.CapturedMessageGlobexWelcomeID), int64(page.Items[0].ID), "newest first")
	assert.Equal(t, int64(fixtures.CapturedMessageGlobexNewsletterID), int64(page.Items[1].ID))
	welcome := page.Items[0]
	assert.Equal(t, siteapi.SiteCapturedMessageSurfaceTransactional, welcome.Surface)
	assert.Equal(t, fixtures.CapturedMessageGlobexWelcomeSubject, welcome.Subject)
	assert.True(t, welcome.FromUnverified)
	assert.True(t, welcome.OutboundMessageId.Null, "a message with no Outbound message names none")
	require.Len(t, empty.Items, 1)
	assert.Equal(t, int64(fixtures.CapturedMessageAcmeArchivedInboxID), int64(empty.Items[0].ID), "a Workspace sees only its own, archived inbox included")
}

func TestSiteCapturedMessagesArePaginated(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)

	page, err := jane.SiteCapturedMessagesList(t.Context(), siteapi.SiteCapturedMessagesListParams{
		WorkspaceId: fixtures.GlobexPublicID, Page: siteapi.NewOptInt32(2), PageSize: siteapi.NewOptInt32(1),
	})
	require.NoError(t, err)

	require.Len(t, page.Items, 1)
	assert.Equal(t, int64(fixtures.CapturedMessageGlobexNewsletterID), int64(page.Items[0].ID))
	assert.Equal(t, int32(2), page.TotalPages)
}

// With no verified Sending domain a send through SMTP is Held, and the same send through the Test
// inbox is Sent and captured with the From as it was, flagged unverified.
func TestTransactionalSendWithoutAVerifiedDomainIsHeldThroughSMTPAndCapturedThroughTheTestInbox(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexID)

	_, held := sendGlobexTransactional(t, env, "held@example.com")

	testhelper.ExternalRefusal(t, held, http.StatusUnprocessableEntity)
	capturedBefore := env.DB.CapturedMessage.Query().CountX(t.Context())
	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexInboxID)

	ok, err := sendGlobexTransactional(t, env, "Ada@Example.com")

	require.NoError(t, err)
	assert.Equal(t, externalapi.TransactionalSendStatusSent, ok.Status)
	kept := env.DB.CapturedMessage.Query().Order(ent.Desc(capturedmessage.FieldID)).FirstX(t.Context())
	assert.Equal(t, capturedBefore+1, env.DB.CapturedMessage.Query().CountX(t.Context()))
	assert.Equal(t, "ada@example.com", kept.Recipient)
	assert.Equal(t, capturedmessage.SurfaceTransactional, kept.Surface)
	assert.True(t, kept.FromUnverified)
	assert.EqualValues(t, fixtures.IntegrationGlobexInboxID, kept.IntegrationID)
	assert.EqualValues(t, fixtures.GlobexID, kept.WorkspaceID)
	assert.Contains(t, kept.HTML, "Globex.")
	require.NotNil(t, kept.OutboundMessageID)
	assert.Equal(t, int64(ok.ID), *kept.OutboundMessageID)
}

func TestTheOutboundMessageOfACapturedSendIsStampedDeliveredOnceAndNothingBounces(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexInboxID)

	sent, err := sendGlobexTransactional(t, env, "ada@example.com")

	require.NoError(t, err)
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
	row := env.DB.OutboundMessage.Query().Where(outboundmessage.ID(int64(sent.ID))).OnlyX(t.Context())
	assert.NotNil(t, row.DeliveredAt)
	assert.Nil(t, row.BouncedAt)
	assert.Nil(t, row.ComplainedAt)
	assert.Equal(t, 1, env.QueuedCount(t, "email.delivered", map[string]any{"outboundMessageId": sent.ID}))
	assert.Zero(t, env.QueuedCount(t, "email.bounced", map[string]any{"outboundMessageId": sent.ID}))
	assert.Zero(t, env.QueuedCount(t, "email.complained", map[string]any{"outboundMessageId": sent.ID}))
}

func TestABroadcastThroughTheTestInboxWithoutAVerifiedDomainIsCaptured(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexInboxID)
	before := env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.GlobexID)).CountX(t.Context())

	_, err := jane.SiteBroadcastsSend(t.Context(), siteapi.SiteBroadcastsSendParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.BroadcastGlobexID),
	})
	require.NoError(t, err)
	env.DeliverQueuedBroadcasts(t)

	after := env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.GlobexID)).CountX(t.Context())
	assert.Equal(t, before+1, after, "the one Contact of Globex that can be mailed received it in the inbox")
	kept := env.DB.CapturedMessage.Query().Where(capturedmessage.Recipient(fixtures.ContactGlobexReaderEmail)).OnlyX(t.Context())
	assert.Equal(t, capturedmessage.SurfaceBroadcast, kept.Surface)
	assert.NotNil(t, kept.SendingSource, "a marketing send carries its Sending source")
	unsubscribed := env.DB.CapturedMessage.Query().Where(capturedmessage.Recipient(fixtures.ContactGlobexEmail)).CountX(t.Context())
	assert.Zero(t, unsubscribed, "a Contact unsubscribed from Broadcasts is skipped as for any Integration")
	status := env.DB.Broadcast.GetX(t.Context(), fixtures.BroadcastGlobexID).Status
	assert.Equal(t, broadcast.StatusSent, status)
}

func TestABroadcastThroughSMTPWithoutAVerifiedDomainIsHeldAndNothingIsCaptured(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexID)
	before := env.DB.CapturedMessage.Query().CountX(t.Context())

	_, err := jane.SiteBroadcastsSend(t.Context(), siteapi.SiteBroadcastsSendParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.BroadcastGlobexID),
	})
	require.NoError(t, err)
	delivered := env.TryDeliverQueuedBroadcasts(t)

	require.ErrorContains(t, delivered, outboundmessages.HoldUnverifiedDomain)
	assert.Equal(t, before, env.DB.CapturedMessage.Query().CountX(t.Context()))
	status := env.DB.Broadcast.GetX(t.Context(), fixtures.BroadcastGlobexID).Status
	assert.NotEqual(t, broadcast.StatusSent, status, "held, not sent")
}

// Globex's Test inbox as its default: its suppressed bounce@globex.test is not sent to, and once
// Globex is suspended nothing is.
func TestSuppressionAndAFreezeApplyToTheTestInboxAsToAnyIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexInboxID)
	before := env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.GlobexID)).CountX(ctx)

	suppressedResult, err := sendGlobexTransactional(t, env, fixtures.SuppressionGlobexDestination)
	require.NoError(t, err)
	suspended, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.GlobexID, suspension.StaffMember("op-42"), "abuse report", time.Now())
	require.NoError(t, err)
	require.True(t, suspended)
	_, frozen := sendGlobexTransactional(t, env, "free@example.com")

	assert.Equal(t, externalapi.TransactionalSendStatusSuppressed, suppressedResult.Status)
	heldResult := testhelper.ExternalRefusal(t, frozen, http.StatusUnprocessableEntity)
	assert.Equal(t, "the message was not sent: "+outboundmessages.Detail(holds.ReasonSuspended), heldResult.Detail.Value)
	after := env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.GlobexID)).CountX(ctx)
	assert.Equal(t, before, after, "neither message reached the inbox")
}

// Disabling the Test inbox stops it capturing; what it kept stays readable until it expires.
func TestDisablingTheTestInboxStopsCapturingAndKeepsWhatWasCaptured(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexInboxID)
	sent, err := sendGlobexTransactional(t, env, "ada@example.com")
	require.NoError(t, err)
	kept := env.DB.CapturedMessage.Query().Where(capturedmessage.OutboundMessageID(int64(sent.ID))).OnlyX(t.Context())
	before := env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.GlobexID)).CountX(t.Context())

	_, err = jane.SiteIntegrationsUpdate(t.Context(), &siteapi.UpdateIntegrationInput{
		Enabled: siteapi.NewOptBool(false),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.IntegrationGlobexInboxID)})
	require.NoError(t, err)
	_, err = sendGlobexTransactional(t, env, "grace@example.com")
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity, "without the Test inbox the send goes to the default, which refuses it")

	after := env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.GlobexID)).CountX(t.Context())
	assert.Equal(t, before, after, "nothing is captured once the Test inbox is disabled")
	_, err = jane.SiteCapturedMessagesGet(t.Context(), siteapi.SiteCapturedMessagesGetParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(kept.ID),
	})
	require.NoError(t, err, "what was captured stays readable")
}
