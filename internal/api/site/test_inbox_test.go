package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/capturedmessage"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func inboxInput() siteapi.SiteIntegrationConfigInput {
	return siteapi.SiteIntegrationConfigInput{
		OneOf: siteapi.NewSiteInboxConfigInputSiteIntegrationConfigInputSum(siteapi.SiteInboxConfigInput{
			Kind: siteapi.SiteInboxConfigInputKindInbox,
		}),
	}
}

func createInbox(t *testing.T, c *siteapi.Client) siteapi.SiteIntegrationsCreateRes {
	t.Helper()
	res, err := c.SiteIntegrationsCreate(context.Background(), &siteapi.SiteCreateIntegrationInput{
		Name: "Test inbox", Config: inboxInput(),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	return res
}

// makeDefault picks an Integration as its Workspace's default, the way any Integration is chosen.
func makeDefault(t *testing.T, c *siteapi.Client, workspace siteapi.WorkspaceId, id int64) {
	t.Helper()
	res, err := c.SiteIntegrationsUpdate(context.Background(), &siteapi.SiteUpdateIntegrationInput{
		IsDefault: siteapi.NewOptBool(true),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: workspace, ID: idStr(id)})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteIntegrationResource{}, res)
}

func sendGlobexTransactional(t *testing.T, env *testhelper.TestEnv, to string) externalapi.EmailsSendRes {
	t.Helper()
	c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.GlobexID, "emails:send"))
	res, err := c.EmailsSend(context.Background(), &externalapi.SendTransactionalEmailInput{
		TemplateId: externalapi.EntityId(fixtures.TemplateGlobexID), Destination: externalapi.EmailAddress(to),
	}, externalapi.EmailsSendParams{})
	require.NoError(t, err)
	return res
}

func TestSiteTestInboxIsCreatedWithoutCredentialsAndIsNotTheDefault(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	out := createInbox(t, c)

	created, ok := out.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "got %T", out)
	assert.Equal(t, siteapi.SiteIntegrationProviderInbox, created.Provider)
	assert.Equal(t, siteapi.SiteIntegrationChannelEmail, created.Channel)
	assert.False(t, created.IsDefault, "a Test inbox never becomes the default by itself")
	kind, ok := created.Config.OneOf.GetSiteInboxConfig()
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteInboxConfigKindInbox, kind.Kind)
	defaults := env.DB.Integration.Query().
		Where(integration.WorkspaceID(fixtures.AcmeID), integration.IsDefault(true)).CountX(t.Context())
	assert.Equal(t, 1, defaults, "the existing default is untouched")
	reread, err := c.SiteIntegrationsGet(t.Context(), siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID})
	require.NoError(t, err)
	limit := reread.(*siteapi.SiteIntegrationResource).SendLimit.PerDay
	assert.Equal(t, int32(200), limit.Limit.Value, "it reports its own ceiling")
	assert.Equal(t, siteapi.NewNilSiteSendLimitSource(siteapi.SiteSendLimitSourceProvider), limit.Source)
}

func TestSiteTestInboxCanBeChosenAsTheDefaultLikeAnyIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	created := createInbox(t, c).(*siteapi.SiteIntegrationResource)

	makeDefault(t, c, fixtures.AcmePublicID, int64(created.ID))

	row := env.DB.Integration.GetX(t.Context(), int64(created.ID))
	assert.True(t, row.IsDefault)
}

func TestSiteSecondTestInboxIsRefusedNamingTheRule(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	require.IsType(t, &siteapi.SiteIntegrationResource{}, createInbox(t, c))

	out := createInbox(t, c)

	refused, ok := out.(*siteapi.SiteIntegrationsCreateConflict)
	require.Truef(t, ok, "got %T", out)
	assert.Contains(t, refused.Detail.Value, "at most one Test inbox")
}

func TestSiteRestoringATestInboxIsRefusedWhileAnotherIsKept(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	first := createInbox(t, c).(*siteapi.SiteIntegrationResource)
	archiveIntegration(t, c, int64(first.ID))
	require.IsType(t, &siteapi.SiteIntegrationResource{}, createInbox(t, c), "the archived one frees the place")

	out := unarchiveIntegration(t, c, int64(first.ID))

	refused, ok := out.(*siteapi.SiteIntegrationsUnarchiveConflict)
	require.Truef(t, ok, "got %T", out)
	assert.Contains(t, refused.Detail.Value, "at most one Test inbox")
}

func TestSiteCapturedMessagesAreListedNewestFirstAndOnlyForTheOwnWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	own, err := jane.SiteCapturedMessagesList(t.Context(), siteapi.SiteCapturedMessagesListParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	other, err := john.SiteCapturedMessagesList(t.Context(), siteapi.SiteCapturedMessagesListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	page, ok := own.(*siteapi.SiteCapturedMessagesListOK)
	require.Truef(t, ok, "got %T", own)
	require.Len(t, page.Items, 2)
	assert.Equal(t, int32(2), page.TotalItems)
	assert.Equal(t, int64(fixtures.CapturedMessageGlobexWelcomeID), int64(page.Items[0].ID), "newest first")
	assert.Equal(t, int64(fixtures.CapturedMessageGlobexNewsletterID), int64(page.Items[1].ID))
	welcome := page.Items[0]
	assert.Equal(t, siteapi.SiteCapturedMessageSurfaceTransactional, welcome.Surface)
	assert.Equal(t, fixtures.CapturedMessageGlobexWelcomeSubject, welcome.Subject)
	assert.True(t, welcome.FromUnverified)
	assert.True(t, welcome.OutboundMessageId.Null, "a message with no Outbound message names none")
	empty, ok := other.(*siteapi.SiteCapturedMessagesListOK)
	require.Truef(t, ok, "got %T", other)
	require.Len(t, empty.Items, 1)
	assert.Equal(t, int64(fixtures.CapturedMessageAcmeArchivedInboxID), int64(empty.Items[0].ID), "a Workspace sees only its own, archived inbox included")
}

func TestSiteCapturedMessagesArePaginated(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)

	out, err := jane.SiteCapturedMessagesList(t.Context(), siteapi.SiteCapturedMessagesListParams{
		WorkspaceId: fixtures.GlobexPublicID, Page: siteapi.NewOptInt32(2), PageSize: siteapi.NewOptInt32(1),
	})
	require.NoError(t, err)

	page := out.(*siteapi.SiteCapturedMessagesListOK)
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

	held := sendGlobexTransactional(t, env, "held@example.com")

	assert.IsType(t, &externalapi.EmailsSendUnprocessableEntity{}, held)
	capturedBefore := env.DB.CapturedMessage.Query().CountX(t.Context())
	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexInboxID)

	sent := sendGlobexTransactional(t, env, "Ada@Example.com")

	ok, isSent := sent.(*externalapi.SendTransactionalEmailResponse)
	require.Truef(t, isSent, "got %T", sent)
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

	sent := sendGlobexTransactional(t, env, "ada@example.com").(*externalapi.SendTransactionalEmailResponse)

	row := env.DB.OutboundMessage.Query().Where(outboundmessage.ID(int64(sent.ID))).OnlyX(t.Context())
	assert.NotNil(t, row.DeliveredAt)
	assert.Nil(t, row.BouncedAt)
	assert.Nil(t, row.ComplainedAt)
	assert.Equal(t, 1, env.OutboxCount(t, "email.delivered", map[string]any{"outboundMessageId": sent.ID}))
	assert.Zero(t, env.OutboxCount(t, "email.bounced", map[string]any{"outboundMessageId": sent.ID}))
	assert.Zero(t, env.OutboxCount(t, "email.complained", map[string]any{"outboundMessageId": sent.ID}))
}

func TestABroadcastThroughTheTestInboxWithoutAVerifiedDomainIsCaptured(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexInboxID)
	before := env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.GlobexID)).CountX(t.Context())

	sent, err := jane.SiteBroadcastsSend(t.Context(), siteapi.SiteBroadcastsSendParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.BroadcastGlobexID),
	})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteBroadcastResource{}, sent)
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

	sent, err := jane.SiteBroadcastsSend(t.Context(), siteapi.SiteBroadcastsSendParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.BroadcastGlobexID),
	})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteBroadcastResource{}, sent)
	delivered := env.TryDeliverQueuedBroadcasts(t)

	require.ErrorContains(t, delivered, outbound.HoldUnverifiedDomain)
	assert.Equal(t, before, env.DB.CapturedMessage.Query().CountX(t.Context()))
	status := env.DB.Broadcast.GetX(t.Context(), fixtures.BroadcastGlobexID).Status
	assert.NotEqual(t, broadcast.StatusSent, status, "held, not sent")
}

func TestSuppressionAndAFreezeApplyToTheTestInboxAsToAnyIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	inbox := createInbox(t, c).(*siteapi.SiteIntegrationResource)
	makeDefault(t, c, fixtures.AcmePublicID, int64(inbox.ID))
	before := env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.AcmeID)).CountX(ctx)
	env.SuppressDestination(ctx, "blocked@example.com")
	external := env.ExternalScoped(t, "emails:send")
	template := externalapi.EntityId(fixtures.TemplateWithVariablesID)

	skipped, err := external.EmailsSend(ctx, &externalapi.SendTransactionalEmailInput{TemplateId: template, Destination: "blocked@example.com"}, externalapi.EmailsSendParams{})
	require.NoError(t, err)
	suspended, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.StaffMember("op-42"), "abuse report")
	require.NoError(t, err)
	require.True(t, suspended)
	frozen, err := external.EmailsSend(ctx, &externalapi.SendTransactionalEmailInput{TemplateId: template, Destination: "free@example.com"}, externalapi.EmailsSendParams{})
	require.NoError(t, err)

	suppressedResult, ok := skipped.(*externalapi.SendTransactionalEmailResponse)
	require.Truef(t, ok, "got %T", skipped)
	assert.Equal(t, externalapi.TransactionalSendStatusSuppressed, suppressedResult.Status)
	heldResult, ok := frozen.(*externalapi.EmailsSendUnprocessableEntity)
	require.Truef(t, ok, "got %T", frozen)
	assert.Equal(t, "the message was not sent: "+outbound.Detail(holds.ReasonSuspended), heldResult.Detail.Value)
	after := env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.AcmeID)).CountX(ctx)
	assert.Equal(t, before, after, "neither message reached the inbox")
}

// Disabling the Test inbox stops it capturing; what it kept stays readable until it expires.
func TestDisablingTheTestInboxStopsCapturingAndKeepsWhatWasCaptured(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	makeDefault(t, jane, fixtures.GlobexPublicID, fixtures.IntegrationGlobexInboxID)
	sent := sendGlobexTransactional(t, env, "ada@example.com").(*externalapi.SendTransactionalEmailResponse)
	kept := env.DB.CapturedMessage.Query().Where(capturedmessage.OutboundMessageID(int64(sent.ID))).OnlyX(t.Context())
	before := env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.GlobexID)).CountX(t.Context())

	disabled, err := jane.SiteIntegrationsUpdate(t.Context(), &siteapi.SiteUpdateIntegrationInput{
		Enabled: siteapi.NewOptBool(false),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.IntegrationGlobexInboxID)})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteIntegrationResource{}, disabled)
	sendGlobexTransactional(t, env, "grace@example.com")

	after := env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.GlobexID)).CountX(t.Context())
	assert.Equal(t, before, after, "nothing is captured once the Test inbox is disabled")
	read, err := jane.SiteCapturedMessagesGet(t.Context(), siteapi.SiteCapturedMessagesGetParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(kept.ID),
	})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteCapturedMessageResource{}, read, "what was captured stays readable")
}
