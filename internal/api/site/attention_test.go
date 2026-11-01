package site_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/integration"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// attentionOf is what needs attention in the Workspace, as the member c reads it.
func attentionOf(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId) []siteapi.SiteAttentionItem {
	t.Helper()
	res, err := c.SiteAttentionGet(t.Context(), siteapi.SiteAttentionGetParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	list, ok := res.(*siteapi.SiteAttentionList)
	require.Truef(t, ok, "got %T", res)
	return list.Items
}

// kinds is the kind of every item, in the order the list gives them.
func kinds(items []siteapi.SiteAttentionItem) []siteapi.SiteAttentionKind {
	return lo.Map(items, func(i siteapi.SiteAttentionItem, _ int) siteapi.SiteAttentionKind { return i.Kind })
}

// itemAbout is the item of the kind about the subject id, and whether there is one.
func itemAbout(items []siteapi.SiteAttentionItem, kind siteapi.SiteAttentionKind, id int64) (siteapi.SiteAttentionItem, bool) {
	return lo.Find(items, func(i siteapi.SiteAttentionItem) bool {
		return i.Kind == kind && i.Subject.Value.ID == siteapi.EntityId(id)
	})
}

func TestAHealthyWorkspaceNeedsNoAttention(t *testing.T) {
	env := testhelper.Setup(t)

	items := attentionOf(t, env.SiteActor(t, fixtures.VandelayOwnerArtEmail), fixtures.VandelayPublicID)

	assert.Empty(t, items)
}

// Fixtures: Acme's Sending domain lost.acme.com lost its DKIM record, and two of its Webhook
// endpoints disabled themselves.
func TestWhatStopsMailComesBeforeWhatFailedAndEveryMemberSeesIt(t *testing.T) {
	env := testhelper.Setup(t)

	items := attentionOf(t, env.SiteActor(t, fixtures.MemberMaryEmail), fixtures.AcmePublicID)

	assert.Equal(t, []siteapi.SiteAttentionKind{
		siteapi.SiteAttentionKindSendingDomainVerificationLost,
		siteapi.SiteAttentionKindWebhookDisabled,
		siteapi.SiteAttentionKindWebhookDisabled,
	}, kinds(items))
	lost := items[0]
	assert.Equal(t, siteapi.SiteAttentionSeverityBlocked, lost.Severity)
	assert.Equal(t, siteapi.SiteAttentionSubject{ID: fixtures.SendingDomainLostID, Name: fixtures.SendingDomainLostDomain}, lost.Subject.Value)
	assert.True(t, lost.Since.Set, "the moment the record was found gone")
	_, disabled := itemAbout(items, siteapi.SiteAttentionKindWebhookDisabled, fixtures.WebhookCodebasicsDisabledID)
	assert.True(t, disabled)
}

func TestAHeldBroadcastSaysWhyButAWorkspaceLevelHoldIsLeftToTheLayout(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, acme.Broadcast().UpdateOneID(fixtures.BroadcastDraftID).SetHoldReason(outbound.HoldUnverifiedDomain).Exec(t.Context()))
	require.NoError(t, acme.Broadcast().UpdateOneID(fixtures.BroadcastAuthoredID).SetHoldReason(holds.ReasonSuspended).Exec(t.Context()))

	items := attentionOf(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID)

	held, ok := itemAbout(items, siteapi.SiteAttentionKindBroadcastHeld, fixtures.BroadcastDraftID)
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteAttentionSeverityBlocked, held.Severity)
	assert.Equal(t, outbound.HoldUnverifiedDomain, held.HoldReason.Value)
	_, suspended := itemAbout(items, siteapi.SiteAttentionKindBroadcastHeld, fixtures.BroadcastAuthoredID)
	assert.False(t, suspended)
}

func TestTheProviderStandingOfAnIntegrationIsAnItem(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, acme.Integration().UpdateOneID(fixtures.IntegrationAcmeDefaultID).
		SetProviderEnforcement(integration.ProviderEnforcementShutdown).Exec(t.Context()))
	require.NoError(t, acme.Integration().UpdateOneID(fixtures.IntegrationAcmeGenericAID).
		SetProviderEnforcement(integration.ProviderEnforcementProbation).SetProviderSandbox(true).Exec(t.Context()))

	items := attentionOf(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID)

	stopped, ok := itemAbout(items, siteapi.SiteAttentionKindIntegrationProviderStopped, fixtures.IntegrationAcmeDefaultID)
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteAttentionSeverityBlocked, stopped.Severity)
	probation, ok := itemAbout(items, siteapi.SiteAttentionKindIntegrationProviderProbation, fixtures.IntegrationAcmeGenericAID)
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteAttentionSeverityDegraded, probation.Severity)
	_, sandbox := itemAbout(items, siteapi.SiteAttentionKindIntegrationProviderSandbox, fixtures.IntegrationAcmeGenericAID)
	assert.True(t, sandbox)
}

// Fixtures: Acme's default Integration sent messages over the last 24 hours.
func TestASpentProviderLimitIsAnItemAndALimitWithRoomIsNot(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	member := env.SiteActor(t, fixtures.OwnerJohnEmail)
	require.NoError(t, acme.Integration().UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerDay(1000).Exec(t.Context()))
	roomy := attentionOf(t, member, fixtures.AcmePublicID)
	require.NoError(t, acme.Integration().UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerDay(1).Exec(t.Context()))

	spent := attentionOf(t, member, fixtures.AcmePublicID)

	_, before := itemAbout(roomy, siteapi.SiteAttentionKindProviderLimitSpent, fixtures.IntegrationAcmeDefaultID)
	_, after := itemAbout(spent, siteapi.SiteAttentionKindProviderLimitSpent, fixtures.IntegrationAcmeDefaultID)
	assert.False(t, before)
	assert.True(t, after)
}

// Fixtures: Acme sent messages over the last 24 hours; the harness runs in the saas Mode, which
// has Send caps.
func TestASpentSendCapIsAnItem(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(1).ExecX(t.Context())

	items := attentionOf(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID)

	capped, ok := lo.Find(items, func(i siteapi.SiteAttentionItem) bool { return i.Kind == siteapi.SiteAttentionKindSendCapSpent })
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteAttentionSeverityDegraded, capped.Severity)
	assert.False(t, capped.Subject.Set, "the Send cap is the Workspace's own")
}

func TestTheRunsOfAnAutomationThatFailedThisWeekAreOneItemWithTheirCount(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	now := time.Now()
	fail := func(id int64, at time.Time) {
		t.Helper()
		require.NoError(t, acme.AutomationRun().UpdateOneID(id).SetStatus(automationrun.StatusFailed).SetEndedAt(at).Exec(t.Context()))
	}
	fail(fixtures.AutomationRunWelcomeCompletedID, now)
	fail(fixtures.AutomationRunWelcomeCompletedSecondID, now)
	fail(fixtures.AutomationRunWelcomeCompletedThirdID, now.AddDate(0, 0, -8))

	items := attentionOf(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID)

	failed, ok := itemAbout(items, siteapi.SiteAttentionKindEnrollmentsFailed, fixtures.AutomationWelcomeSeriesID)
	require.True(t, ok)
	assert.EqualValues(t, 2, failed.Count, "the run of eight days ago is no longer an item")
	assert.Equal(t, siteapi.SiteAttentionSeverityNotice, failed.Severity)
}

// Fixtures: Acme's failed Broadcast failed long ago.
func TestABroadcastThatFailedThisWeekIsAnItem(t *testing.T) {
	env := testhelper.Setup(t)
	member := env.SiteActor(t, fixtures.OwnerJohnEmail)
	old := attentionOf(t, member, fixtures.AcmePublicID)
	require.NoError(t, env.DB.Scoped(fixtures.AcmeID).Broadcast().UpdateOneID(fixtures.BroadcastFailedID).SetUpdatedAt(time.Now()).Exec(t.Context()))

	recent := attentionOf(t, member, fixtures.AcmePublicID)

	_, before := itemAbout(old, siteapi.SiteAttentionKindBroadcastFailed, fixtures.BroadcastFailedID)
	_, after := itemAbout(recent, siteapi.SiteAttentionKindBroadcastFailed, fixtures.BroadcastFailedID)
	assert.False(t, before)
	assert.True(t, after)
}
