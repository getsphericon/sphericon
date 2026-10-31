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

func listIntegrations(t *testing.T, c *siteapi.Client, archived bool) []siteapi.SiteIntegrationResource {
	t.Helper()
	res, err := c.SiteIntegrationsList(context.Background(), siteapi.SiteIntegrationsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		Archived:    siteapi.NewOptBool(archived),
	})
	require.NoError(t, err)
	list, ok := res.(*siteapi.SiteIntegrationsListOKApplicationJSON)
	require.Truef(t, ok, "got %T", res)
	return *list
}

func integrationIDs(items []siteapi.SiteIntegrationResource) []int64 {
	return lo.Map(items, func(i siteapi.SiteIntegrationResource, _ int) int64 { return int64(i.ID) })
}

func archiveIntegration(t *testing.T, c *siteapi.Client, id int64) siteapi.SiteIntegrationsArchiveRes {
	t.Helper()
	got, err := c.SiteIntegrationsArchive(context.Background(), siteapi.SiteIntegrationsArchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
	require.NoError(t, err)
	return got
}

func unarchiveIntegration(t *testing.T, c *siteapi.Client, id int64) siteapi.SiteIntegrationsUnarchiveRes {
	t.Helper()
	got, err := c.SiteIntegrationsUnarchive(context.Background(), siteapi.SiteIntegrationsUnarchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
	require.NoError(t, err)
	return got
}

// An Integration is archived, not deleted (ADR 0049): it leaves the default list, is listed by
// archived=true, stays openable by id, and comes back when restored.
func TestSiteIntegrationsArchiveAndRestore(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := int64(fixtures.IntegrationAcmeSesID)

	out := archiveIntegration(t, c, id)
	archived, ok := out.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "got %T", out)
	assert.True(t, archived.ArchivedAt.IsSet())

	assert.NotContains(t, integrationIDs(listIntegrations(t, c, false)), id)
	shown := integrationIDs(listIntegrations(t, c, true))
	assert.Contains(t, shown, id)
	assert.Contains(t, shown, int64(fixtures.IntegrationAcmeArchivedID))
	assert.NotContains(t, shown, int64(fixtures.IntegrationAcmeDefaultID))

	got, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
	require.NoError(t, err)
	opened, ok := got.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "an archived Integration can still be opened, got %T", got)
	assert.True(t, opened.ArchivedAt.IsSet())

	back := unarchiveIntegration(t, c, id)
	restored, ok := back.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "got %T", back)
	assert.False(t, restored.ArchivedAt.IsSet())
	assert.Contains(t, integrationIDs(listIntegrations(t, c, false)), id)

	missing := archiveIntegration(t, c, 999999)
	assert.IsType(t, &siteapi.SiteIntegrationsArchiveNotFound{}, missing)
}

// Archiving the default Integration clears its default flag, and restoring it does not give the
// flag back: the channel is left with no default until someone picks one. Umbrella has no live
// Broadcast or Automation, so nothing refuses the archive. Sam is the Umbrella admin whose
// Second factor satisfies its Two-factor requirement.
func TestSiteIntegrationsArchivingTheDefaultClearsItAndRestoreDoesNotReturnIt(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.SecondFactorSamEmail)
	ctx := context.Background()
	id := idStr(fixtures.IntegrationUmbrellaDefaultID)
	const workspace = fixtures.UmbrellaPublicID

	out, err := c.SiteIntegrationsArchive(ctx, siteapi.SiteIntegrationsArchiveParams{WorkspaceId: workspace, ID: id})
	require.NoError(t, err)
	archived, ok := out.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "got %T", out)
	assert.False(t, archived.IsDefault)
	assert.True(t, archived.ArchivedAt.IsSet())

	back, err := c.SiteIntegrationsUnarchive(ctx, siteapi.SiteIntegrationsUnarchiveParams{WorkspaceId: workspace, ID: id})
	require.NoError(t, err)
	restored, ok := back.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "got %T", back)
	assert.False(t, restored.IsDefault, "restoring never makes an Integration the default again")

	listed, err := c.SiteIntegrationsList(ctx, siteapi.SiteIntegrationsListParams{WorkspaceId: workspace})
	require.NoError(t, err)
	list, ok := listed.(*siteapi.SiteIntegrationsListOKApplicationJSON)
	require.Truef(t, ok, "got %T", listed)
	defaults := lo.Filter(*list, func(i siteapi.SiteIntegrationResource, _ int) bool { return i.IsDefault })
	assert.Empty(t, defaults)

	// The restored Integration can be promoted by hand.
	promoted, err := c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{IsDefault: siteapi.NewOptBool(true)},
		siteapi.SiteIntegrationsUpdateParams{WorkspaceId: workspace, ID: id})
	require.NoError(t, err)
	assert.True(t, promoted.(*siteapi.SiteIntegrationResource).IsDefault)
}

// An archived Integration cannot be made the default: restore it first.
func TestSiteIntegrationsArchivedOneCannotBecomeTheDefault(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	got, err := c.SiteIntegrationsUpdate(context.Background(), &siteapi.UpdateIntegrationInput{IsDefault: siteapi.NewOptBool(true)},
		siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.IntegrationAcmeArchivedID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteIntegrationsUpdateUnprocessableEntity{}, got)
}

// A sent Broadcast's report keeps naming the Integration it went through after that Integration
// is archived, and the Integration is still readable by id.
func TestSiteSentBroadcastStillNamesItsArchivedIntegration(t *testing.T) {
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
	assert.True(t, row.ArchivedAt.IsSet())
}

// Archive and restore are recorded in the Audit log with their actor.
func TestSiteIntegrationsArchiveIsAudited(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	require.IsType(t, &siteapi.SiteIntegrationResource{}, archiveIntegration(t, c, fixtures.IntegrationAcmeSesID))
	require.IsType(t, &siteapi.SiteIntegrationResource{}, unarchiveIntegration(t, c, fixtures.IntegrationAcmeSesID))

	entries := lo.Map(env.OutboxEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) *events.AuditEntry {
		entry, ok := ev.(*events.AuditEntry)
		require.True(t, ok)
		return entry
	})
	actions := lo.Map(entries, func(e *events.AuditEntry, _ int) string { return e.Action })
	assert.Equal(t, []string{"integration.discard", "integration.restore"}, actions)
	assert.Equal(t, events.ActorUser, entries[0].Actor.Kind)
}

// Archiving and restoring, like every other write, needs an Owner or an Admin.
func TestSiteIntegrationsArchiveNeedsAnOwnerOrAdmin(t *testing.T) {
	env := testhelper.Setup(t)
	member := env.SiteActor(t, fixtures.MemberMaryEmail)

	assert.IsType(t, new(siteapi.SiteIntegrationsArchiveForbidden), archiveIntegration(t, member, fixtures.IntegrationAcmeSesID))
	assert.IsType(t, new(siteapi.SiteIntegrationsUnarchiveForbidden), unarchiveIntegration(t, member, fixtures.IntegrationAcmeArchivedID))
}
