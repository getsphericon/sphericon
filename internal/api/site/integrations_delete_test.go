package site_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func listIntegrations(t *testing.T, c *siteapi.Client) []siteapi.SiteIntegrationResource {
	t.Helper()
	res, err := c.SiteIntegrationsList(context.Background(), siteapi.SiteIntegrationsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	list, ok := res.(*siteapi.SiteIntegrationsListOKApplicationJSON)
	require.Truef(t, ok, "got %T", res)
	return *list
}

func integrationIDs(items []siteapi.SiteIntegrationResource) []int64 {
	return lo.Map(items, func(i siteapi.SiteIntegrationResource, _ int) int64 { return int64(i.ID) })
}

func deleteIntegration(t *testing.T, c *siteapi.Client, id int64) siteapi.SiteIntegrationsDeleteRes {
	t.Helper()
	got, err := c.SiteIntegrationsDelete(context.Background(), siteapi.SiteIntegrationsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
	require.NoError(t, err)
	return got
}

// A deleted Integration is discarded, not removed physically (ADR 0049): it leaves the list for
// good and stays readable by id, marked deleted. Deleting it again changes nothing.
func TestSiteIntegrationsDelete(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := int64(fixtures.IntegrationAcmeSesID)

	assert.IsType(t, &siteapi.SiteIntegrationsDeleteNoContent{}, deleteIntegration(t, c, id))

	listed := integrationIDs(listIntegrations(t, c))
	assert.NotContains(t, listed, id)
	assert.NotContains(t, listed, int64(fixtures.IntegrationAcmeArchivedID))
	assert.Contains(t, listed, int64(fixtures.IntegrationAcmeDefaultID))

	got, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
	require.NoError(t, err)
	opened, ok := got.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "a deleted Integration can still be read by id, got %T", got)
	assert.True(t, opened.DiscardedAt.IsSet())

	assert.IsType(t, &siteapi.SiteIntegrationsDeleteNoContent{}, deleteIntegration(t, c, id), "deleting it again changes nothing")
	assert.IsType(t, &siteapi.SiteIntegrationsDeleteNotFound{}, deleteIntegration(t, c, 999999))
}

// Deleting the default Integration clears its default flag: the channel is left with no default
// until someone picks one. Umbrella has no live Broadcast or Automation, so nothing refuses the
// delete. Sam is the Umbrella admin whose Second factor satisfies its Two-factor requirement.
func TestSiteIntegrationsDeletingTheDefaultLeavesTheChannelWithout(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.SecondFactorSamEmail)
	ctx := context.Background()
	id := idStr(fixtures.IntegrationUmbrellaDefaultID)
	const workspace = fixtures.UmbrellaPublicID

	out, err := c.SiteIntegrationsDelete(ctx, siteapi.SiteIntegrationsDeleteParams{WorkspaceId: workspace, ID: id})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteIntegrationsDeleteNoContent{}, out)

	got, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: workspace, ID: id})
	require.NoError(t, err)
	deleted, ok := got.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "got %T", got)
	assert.False(t, deleted.IsDefault)

	listed, err := c.SiteIntegrationsList(ctx, siteapi.SiteIntegrationsListParams{WorkspaceId: workspace})
	require.NoError(t, err)
	list, ok := listed.(*siteapi.SiteIntegrationsListOKApplicationJSON)
	require.Truef(t, ok, "got %T", listed)
	defaults := lo.Filter(*list, func(i siteapi.SiteIntegrationResource, _ int) bool { return i.IsDefault })
	assert.Empty(t, defaults)
}

// A deleted Integration is read, never changed (ADR 0100): an edit, a promotion to the default and
// a new hook key answer 404, and Get still answers.
func TestSiteIntegrationsDeletedOneIsReadOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := idStr(fixtures.IntegrationAcmeArchivedID)
	path := siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: id}

	renamed, err := c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{Name: siteapi.NewOptString("Renamed")}, path)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteIntegrationsUpdateNotFound{}, renamed)

	promoted, err := c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{IsDefault: siteapi.NewOptBool(true)}, path)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteIntegrationsUpdateNotFound{}, promoted)

	rekeyed, err := c.SiteIntegrationsRegenerateKey(ctx, siteapi.SiteIntegrationsRegenerateKeyParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteIntegrationsRegenerateKeyNotFound{}, rekeyed)

	got, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	row, ok := got.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "got %T", got)
	assert.Equal(t, "Old relay (archived)", row.Name, "the refused edit left it as it was")
}

// A sent Broadcast's report keeps naming the Integration it went through after that Integration
// is deleted, and the Integration is still readable by id.
func TestSiteSentBroadcastStillNamesItsDeletedIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	got, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.BroadcastSentViaArchivedIntegrationID})
	require.NoError(t, err)
	b, ok := got.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", got)
	id, named := b.IntegrationId.Get()
	require.True(t, named)
	assert.EqualValues(t, fixtures.IntegrationAcmeArchivedID, id)

	integ, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	row, ok := integ.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "got %T", integ)
	assert.Equal(t, "Old relay (archived)", row.Name)
	assert.True(t, row.DiscardedAt.IsSet())
}

// The delete is recorded in the Audit log with its actor.
func TestSiteIntegrationsDeleteIsAudited(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	require.IsType(t, &siteapi.SiteIntegrationsDeleteNoContent{}, deleteIntegration(t, c, fixtures.IntegrationAcmeSesID))

	entries := lo.Map(env.OutboxEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) *events.AuditEntry {
		entry, ok := ev.(*events.AuditEntry)
		require.True(t, ok)
		return entry
	})
	actions := lo.Map(entries, func(e *events.AuditEntry, _ int) string { return e.Action })
	assert.Equal(t, []string{"integration.discard"}, actions)
	assert.Equal(t, events.ActorUser, entries[0].Actor.Kind)
}

// Deleting, like every other write, needs an Owner or an Admin.
func TestSiteIntegrationsDeleteNeedsAnOwnerOrAdmin(t *testing.T) {
	env := testhelper.Setup(t)
	member := env.SiteActor(t, fixtures.MemberMaryEmail)

	assert.IsType(t, new(siteapi.SiteIntegrationsDeleteForbidden), deleteIntegration(t, member, fixtures.IntegrationAcmeSesID))
}

// A stored empty definition means no Steps to Publish and Needs attention. Removal must make
// the same decision: this Automation does not need its otherwise disabled Integration.
func TestAnEmptyStoredDefinitionAllowsPublishingAndRemovingItsUnusedIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	stopMailgunSender(t, s)
	err := s.AutomationVersion().UpdateOneID(fixtures.AutomationHoldDemoVersionID).
		SetSteps("").Exec(ctx)
	require.NoError(t, err)
	err = s.Integration().UpdateOneID(fixtures.IntegrationAcmeMailgunID).
		SetEnabled(false).Exec(ctx)
	require.NoError(t, err)

	published, err := c.SiteAutomationsPublish(ctx, siteapi.SiteAutomationsPublishParams{
		WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AutomationHoldDemoID),
	})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteAutomationResource{}, published)
	items := attentionOf(t, c, fixtures.AcmePublicID)
	_, needsAttention := cannotSendOf(items, fixtures.AutomationHoldDemoID)
	assert.False(t, needsAttention, "no Step sends through the disabled Integration")

	require.NoError(t, env.Integrations.Delete(ctx, s, fixtures.IntegrationAcmeMailgunID), "an empty stored definition must not disagree with Publish")
	deleted, err := env.Integrations.Get(ctx, s, fixtures.IntegrationAcmeMailgunID)
	require.NoError(t, err)
	assert.NotNil(t, deleted.DiscardedAt)
}
