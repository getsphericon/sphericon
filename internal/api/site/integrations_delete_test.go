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
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/integrationdeletion"
)

func listIntegrations(t *testing.T, c *siteapi.Client) []siteapi.SiteIntegrationResource {
	t.Helper()
	list, err := c.SiteIntegrationsList(context.Background(), siteapi.SiteIntegrationsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	return list
}

func integrationIDs(items []siteapi.SiteIntegrationResource) []int64 {
	return lo.Map(items, func(i siteapi.SiteIntegrationResource, _ int) int64 { return int64(i.ID) })
}

func deleteIntegration(t *testing.T, c *siteapi.Client, id int64) error {
	t.Helper()
	return deleteIntegrationOf(t, c, fixtures.AcmePublicID, id)
}

// deleteIntegrationOf deletes the Integration id of the Workspace as c: nil, or the refusal.
func deleteIntegrationOf(t *testing.T, c *siteapi.Client, ws siteapi.WorkspaceId, id int64) error {
	t.Helper()
	return c.SiteIntegrationsDelete(t.Context(), siteapi.SiteIntegrationsDeleteParams{WorkspaceId: ws, ID: idStr(id)})
}

// A deleted Integration is discarded, not removed physically (ADR 0049): it leaves the list for
// good and stays readable by id, marked deleted. Deleting it again changes nothing.
func TestSiteIntegrationsDelete(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := int64(fixtures.IntegrationAcmeSesID)

	require.NoError(t, deleteIntegration(t, c, id))

	listed := integrationIDs(listIntegrations(t, c))
	assert.NotContains(t, listed, id)
	assert.NotContains(t, listed, int64(fixtures.IntegrationAcmeArchivedID))
	assert.Contains(t, listed, int64(fixtures.IntegrationAcmeDefaultID))

	opened, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
	require.NoError(t, err)
	assert.True(t, opened.DiscardedAt.IsSet())

	require.NoError(t, deleteIntegration(t, c, id), "deleting it again changes nothing")
	testhelper.SiteRefusal(t, deleteIntegration(t, c, 999999), http.StatusNotFound)
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

	err := c.SiteIntegrationsDelete(ctx, siteapi.SiteIntegrationsDeleteParams{WorkspaceId: workspace, ID: id})
	require.NoError(t, err)

	deleted, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: workspace, ID: id})
	require.NoError(t, err)
	assert.False(t, deleted.IsDefault)

	list, err := c.SiteIntegrationsList(ctx, siteapi.SiteIntegrationsListParams{WorkspaceId: workspace})
	require.NoError(t, err)
	defaults := lo.Filter(list, func(i siteapi.SiteIntegrationResource, _ int) bool { return i.IsDefault })
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

	_, err := c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{Name: siteapi.NewOptString("Renamed")}, path)
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	_, err = c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{IsDefault: siteapi.NewOptBool(true)}, path)
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	_, err = c.SiteIntegrationsRegenerateKey(ctx, siteapi.SiteIntegrationsRegenerateKeyParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	row, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	assert.Equal(t, "Old relay (archived)", row.Name, "the refused edit left it as it was")
}

// A sent Broadcast's report keeps naming the Integration it went through after that Integration
// is deleted, and the Integration is still readable by id.
func TestSiteSentBroadcastStillNamesItsDeletedIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	b, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.BroadcastSentViaArchivedIntegrationID})
	require.NoError(t, err)
	id, named := b.IntegrationId.Get()
	require.True(t, named)
	assert.EqualValues(t, fixtures.IntegrationAcmeArchivedID, id)

	row, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	assert.Equal(t, "Old relay (archived)", row.Name)
	assert.True(t, row.DiscardedAt.IsSet())
}

// The delete is recorded in the Audit log with its actor.
func TestSiteIntegrationsDeleteIsAudited(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	require.NoError(t, deleteIntegration(t, c, fixtures.IntegrationAcmeSesID))

	entries := lo.Map(env.QueuedEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) *events.AuditEntry {
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

	testhelper.SiteRefusal(t, deleteIntegration(t, member, fixtures.IntegrationAcmeSesID), http.StatusForbidden)
}

// A stored empty definition means no Steps to Publish and Needs attention. Removal must make
// the same decision: this Automation does not need the disabled Integration nothing names.
func TestAnEmptyStoredDefinitionAllowsPublishingAndRemovingItsUnusedIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.MeridianID)
	c := env.SiteActor(t, fixtures.MeridianOwnerMonaEmail)

	_, err := c.SiteAutomationsPublish(ctx, siteapi.SiteAutomationsPublishParams{
		WorkspaceId: fixtures.MeridianPublicID, ID: idStr(fixtures.AutomationMeridianEmptyID),
	})
	require.NoError(t, err)
	items := attentionOf(t, c, fixtures.MeridianPublicID)
	_, needsAttention := cannotSendOf(items, fixtures.AutomationMeridianEmptyID)
	assert.False(t, needsAttention, "no Step sends through the disabled Integration")

	require.NoError(t, integrationdeletion.New(env.Bus, env.Integrations).Delete(ctx, s, fixtures.IntegrationMeridianDisabledID), "an empty stored definition must not disagree with Publish")
	deleted, err := env.Integrations.Get(ctx, s, fixtures.IntegrationMeridianDisabledID)
	require.NoError(t, err)
	assert.NotNil(t, deleted.DiscardedAt)
}
