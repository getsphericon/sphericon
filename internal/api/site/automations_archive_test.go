package site_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/broadcast"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func listAutomations(t *testing.T, c *siteapi.Client, archived bool) []int64 {
	t.Helper()
	res, err := c.SiteAutomationsList(context.Background(), siteapi.SiteAutomationsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		PageSize:    siteapi.NewOptInt32(100),
		Archived:    siteapi.NewOptBool(archived),
	})
	require.NoError(t, err)
	list, ok := res.(*siteapi.SiteAutomationsListOK)
	require.Truef(t, ok, "got %T", res)
	return lo.Map(list.Items, func(a siteapi.SiteAutomationResource, _ int) int64 { return int64(a.ID) })
}

func archiveAutomation(c *siteapi.Client, id int64) (siteapi.SiteAutomationsArchiveRes, error) {
	return c.SiteAutomationsArchive(context.Background(), siteapi.SiteAutomationsArchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)})
}

// An Automation with active Enrollments is refused with 409 and their count; the Automation
// stays in the list.
func TestSiteAutomationsArchiveRefusedWithActiveEnrollments(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	out, err := archiveAutomation(c, fixtures.AutomationWelcomeSeriesID)
	require.NoError(t, err)
	refused, ok := out.(*siteapi.InUseProblemDetails)
	require.Truef(t, ok, "got %T", out)
	require.Len(t, refused.UsedBy, 1)
	assert.Equal(t, siteapi.UsedByKindEnrollment, refused.UsedBy[0].Kind)
	assert.EqualValues(t, fixtures.AutomationWelcomeSeriesID, refused.UsedBy[0].ID)
	assert.Equal(t, fixtures.AutomationWelcomeSeriesName, refused.UsedBy[0].Name)
	assert.EqualValues(t, 2, refused.UsedBy[0].Count.Or(0))

	assert.Contains(t, listAutomations(t, c, false), int64(fixtures.AutomationWelcomeSeriesID))
}

// With no active Enrollment the Automation is archived: it leaves the default list, shows with
// archived=true, can still be opened, and comes back when restored.
func TestSiteAutomationsArchiveAndRestore(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := int64(fixtures.AutomationNotifySlackID) // its only Enrollment has failed

	out, err := archiveAutomation(c, id)
	require.NoError(t, err)
	archived, ok := out.(*siteapi.SiteAutomationResource)
	require.Truef(t, ok, "got %T", out)
	assert.True(t, archived.ArchivedAt.IsSet())

	assert.NotContains(t, listAutomations(t, c, false), id)
	shown := listAutomations(t, c, true)
	assert.Contains(t, shown, id)
	assert.Contains(t, shown, int64(fixtures.AutomationArchivedID))
	assert.NotContains(t, shown, int64(fixtures.AutomationWelcomeSeriesID))

	got, err := c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)})
	require.NoError(t, err)
	assert.IsTypef(t, &siteapi.SiteAutomationResource{}, got, "an archived automation can still be opened")

	back, err := c.SiteAutomationsUnarchive(ctx, siteapi.SiteAutomationsUnarchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)})
	require.NoError(t, err)
	restored, ok := back.(*siteapi.SiteAutomationResource)
	require.Truef(t, ok, "got %T", back)
	assert.False(t, restored.ArchivedAt.IsSet())
	assert.Contains(t, listAutomations(t, c, false), id)
}

// An archived Automation enrolls nobody, even though its status is active.
func TestArchivedAutomationEnrollsNobody(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	id := int64(fixtures.AutomationHoldDemoID) // active, trigger hold_demo, no Enrollment

	_, err := archiveAutomation(c, id)
	require.NoError(t, err)
	enrolled, err := jobs.EvaluateTrigger(ctx, acme, fixtures.ContactAliceID, "hold_demo")
	require.NoError(t, err)
	assert.Empty(t, enrolled)

	_, err = c.SiteAutomationsUnarchive(ctx, siteapi.SiteAutomationsUnarchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)})
	require.NoError(t, err)
	enrolled, err = jobs.EvaluateTrigger(ctx, acme, fixtures.ContactAliceID, "hold_demo")
	require.NoError(t, err)
	assert.Len(t, enrolled, 1)
}

// Delete purges an Automation with no Enrollments; one with any Enrollment is history and
// answers 422, whatever the status of its Enrollments.
func TestSiteAutomationsDeleteOnlyPurgesUnenrolled(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	del := func(id int64) (siteapi.SiteAutomationsDeleteRes, error) {
		return c.SiteAutomationsDelete(ctx, siteapi.SiteAutomationsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)})
	}

	gone, err := del(fixtures.AutomationWinBackID)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAutomationsDeleteNoContent{}, gone)
	exists, err := env.DB.Automation.Query().Where(automation.ID(fixtures.AutomationWinBackID)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists, "purged for good")

	purged, err := del(fixtures.AutomationArchivedID)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAutomationsDeleteNoContent{}, purged, "an archived automation with no Enrollments is purged too")

	for _, id := range []int64{fixtures.AutomationWelcomeSeriesID, fixtures.AutomationNotifySlackID} {
		out, err := del(id)
		require.NoError(t, err)
		assert.IsType(t, &siteapi.InUseProblemDetails{}, out, "automation %d: its Enrollments are history, so 409", id)
	}

	missing, err := del(999999)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAutomationsDeleteNotFound{}, missing)
}

// Archive and restore are recorded in the Audit log with their actor.
func TestSiteAutomationsArchiveIsAudited(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := archiveAutomation(c, fixtures.AutomationNotifySlackID)
	require.NoError(t, err)
	_, err = c.SiteAutomationsUnarchive(context.Background(), siteapi.SiteAutomationsUnarchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.AutomationNotifySlackID})
	require.NoError(t, err)

	entries := lo.Map(env.OutboxEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) *events.AuditEntry {
		entry, ok := ev.(*events.AuditEntry)
		require.True(t, ok)
		return entry
	})
	actions := lo.Map(entries, func(e *events.AuditEntry, _ int) string { return e.Action })
	assert.Equal(t, []string{"automation.discard", "automation.restore"}, actions)
	assert.Equal(t, events.ActorUser, entries[0].Actor.Kind)
	assert.Equal(t, "Notify Slack on signup", entries[0].TargetName)
}

// An archived Automation is not live work: it no longer holds the default Integration.
func TestArchivedAutomationDoesNotUseTheDefaultIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	del := func() siteapi.SiteIntegrationsDeleteRes {
		got, err := c.SiteIntegrationsDelete(ctx, siteapi.SiteIntegrationsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.IntegrationAcmeDefaultID)})
		require.NoError(t, err)
		return got
	}
	_, err := acme.Broadcast().Update().Where(broadcast.StatusIn(broadcast.StatusScheduled, broadcast.StatusSending)).SetStatus(broadcast.StatusSent).SetSentAt(time.Now()).Save(ctx)
	require.NoError(t, err)
	refused, ok := del().(*siteapi.InUseProblemDetails)
	require.True(t, ok)
	names := lo.Map(refused.UsedBy, func(u siteapi.UsedBy, _ int) string { return u.Name })
	require.Contains(t, names, fixtures.AutomationWelcomeSeriesName)

	for _, id := range acme.Automation().Query().Where(automation.StatusEQ(automation.StatusActive)).IDsX(ctx) {
		require.NoError(t, acme.Automation().Discard(ctx, id))
	}

	assert.IsType(t, &siteapi.SiteIntegrationsDeleteNoContent{}, del())
}
