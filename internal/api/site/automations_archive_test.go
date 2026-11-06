package site_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/stepping"
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
	list := res
	return lo.Map(list.Items, func(a siteapi.SiteAutomationResource, _ int) int64 { return int64(a.ID) })
}

func archiveAutomation(c *siteapi.Client, id int64) (*siteapi.SiteAutomationResource, error) {
	return c.SiteAutomationsArchive(context.Background(), siteapi.SiteAutomationsArchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)})
}

// An Automation with active Enrollments is refused with 409 and their count; the Automation
// stays in the list.
func TestSiteAutomationsArchiveRefusedWithActiveEnrollments(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := archiveAutomation(c, fixtures.AutomationWelcomeSeriesID)
	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	require.Len(t, refused.UsedBy, 1)
	assert.Equal(t, siteapi.UsedByKindEnrollment, refused.UsedBy[0].Kind)
	assert.EqualValues(t, fixtures.AutomationWelcomeSeriesID, refused.UsedBy[0].ID)
	assert.Equal(t, fixtures.AutomationWelcomeSeriesName, refused.UsedBy[0].Name)
	assert.EqualValues(t, 2, refused.UsedBy[0].Count.Or(0))

	assert.Contains(t, listAutomations(t, c, false), int64(fixtures.AutomationWelcomeSeriesID))
}

// With no active Enrollment the Automation is archived: it leaves the default list, shows with
// archived=true, can still be opened, and comes back when unarchived.
func TestSiteAutomationsArchiveAndUnarchive(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := int64(fixtures.AutomationNotifySlackID) // its only Enrollment has failed

	out, err := archiveAutomation(c, id)
	require.NoError(t, err)
	archived := out
	assert.True(t, archived.ArchivedAt.IsSet())

	assert.NotContains(t, listAutomations(t, c, false), id)
	shown := listAutomations(t, c, true)
	assert.Contains(t, shown, id)
	assert.Contains(t, shown, int64(fixtures.AutomationArchivedID))
	assert.NotContains(t, shown, int64(fixtures.AutomationWelcomeSeriesID))

	_, err = c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)})
	require.NoError(t, err)

	back, err := c.SiteAutomationsUnarchive(ctx, siteapi.SiteAutomationsUnarchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)})
	require.NoError(t, err)
	restored := back
	assert.False(t, restored.ArchivedAt.IsSet())
	assert.Contains(t, listAutomations(t, c, false), id)
}

// An archived Automation enrolls nobody: archiving deactivates it, so restoring it does not re-arm
// it either. Only Publish makes it enroll again.
func TestArchivedAutomationEnrollsNobody(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	id := int64(fixtures.AutomationHoldDemoID) // active, trigger hold_demo, no Enrollment

	_, err := archiveAutomation(c, id)
	require.NoError(t, err)
	require.NoError(t, stepping.OnEvent(ctx, env.Bus, acme, fixtures.ContactAliceID, automations.Event{Action: "hold_demo", OccurredAt: time.Now()}, 0))
	assert.Empty(t, env.JobsOf(t, stepping.RunStepArgs{}.Kind()), "an archived Automation starts no Enrollment")

	_, err = c.SiteAutomationsUnarchive(ctx, siteapi.SiteAutomationsUnarchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)})
	require.NoError(t, err)
	require.NoError(t, stepping.OnEvent(ctx, env.Bus, acme, fixtures.ContactAliceID, automations.Event{Action: "hold_demo", OccurredAt: time.Now()}, 0))
	assert.Empty(t, env.JobsOf(t, stepping.RunStepArgs{}.Kind()), "the unarchived Automation is inactive")

	_, err = c.SiteAutomationsPublish(ctx, siteapi.SiteAutomationsPublishParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)})
	require.NoError(t, err)
	require.NoError(t, stepping.OnEvent(ctx, env.Bus, acme, fixtures.ContactAliceID, automations.Event{Action: "hold_demo", OccurredAt: time.Now()}, 0))
	assert.Len(t, env.JobsOf(t, stepping.RunStepArgs{}.Kind()), 1, "the published Automation enrolls the Contact")
}

// An Automation is archived, never deleted (ADR 0100): one never published and one that has sent
// mail are both archived, and both stay readable by id.
func TestSiteAutomationsArchiveKeepsEveryAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	for _, id := range []int64{fixtures.AutomationWinBackID, fixtures.AutomationOnOpenID} {
		out, err := archiveAutomation(c, id)
		require.NoError(t, err)
		archived := out
		assert.True(t, archived.ArchivedAt.IsSet())

		_, err = c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)})
		require.NoError(t, err)
	}
	shown := listAutomations(t, c, true)
	assert.Contains(t, shown, int64(fixtures.AutomationWinBackID))
	assert.Contains(t, shown, int64(fixtures.AutomationOnOpenID))
}

// An archived Automation is read-only until it is unarchived (ADR 0100): an edit, Publish, Revert
// and Deactivate answer 409, and its name and draft stay as they were.
func TestSiteAnArchivedAutomationIsReadOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteAutomationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.AutomationArchivedID)}
	before, err := c.SiteAutomationsGet(ctx, params)
	require.NoError(t, err)

	_, edited := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{Name: siteapi.NewOptString("Renamed")},
		siteapi.SiteAutomationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.AutomationArchivedID)})
	_, published := c.SiteAutomationsPublish(ctx, siteapi.SiteAutomationsPublishParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.AutomationArchivedID)})
	_, reverted := c.SiteAutomationsRevert(ctx, siteapi.SiteAutomationsRevertParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.AutomationArchivedID)})
	_, deactivated := c.SiteAutomationsDeactivate(ctx, siteapi.SiteAutomationsDeactivateParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.AutomationArchivedID)})
	after, err := c.SiteAutomationsGet(ctx, params)
	require.NoError(t, err)

	testhelper.SiteRefusal(t, edited, http.StatusConflict)
	testhelper.SiteRefusal(t, published, http.StatusConflict)
	testhelper.SiteRefusal(t, reverted, http.StatusConflict)
	testhelper.SiteRefusal(t, deactivated, http.StatusConflict)
	assert.Equal(t, before, after)
}

// Archive and Unarchive are recorded in the Audit log with their actor, by their own names.
func TestSiteAutomationsArchiveIsAudited(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := archiveAutomation(c, fixtures.AutomationNotifySlackID)
	require.NoError(t, err)
	_, err = c.SiteAutomationsUnarchive(context.Background(), siteapi.SiteAutomationsUnarchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.AutomationNotifySlackID})
	require.NoError(t, err)

	entries := lo.Map(env.QueuedEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) *events.AuditEntry {
		entry, ok := ev.(*events.AuditEntry)
		require.True(t, ok)
		return entry
	})
	actions := lo.Map(entries, func(e *events.AuditEntry, _ int) string { return e.Action })
	assert.Equal(t, []string{"automation.archive", "automation.unarchive"}, actions)
	assert.Equal(t, events.ActorUser, entries[0].Actor.Kind)
	assert.Equal(t, "Notify Slack on signup", entries[0].TargetName)
}

// An archived Automation no Enrollment stands on is not live work: it no longer holds the default
// Integration.
func TestArchivedAutomationDoesNotUseTheDefaultIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MeridianOwnerMonaEmail)
	meridian := env.DB.Scoped(fixtures.MeridianID)
	del := func() error {
		return deleteIntegrationOf(t, c, fixtures.MeridianPublicID, fixtures.IntegrationMeridianDefaultID)
	}
	sendMeridianBroadcasts(t, meridian)
	refused := testhelper.SiteRefusal(t, del(), http.StatusConflict)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindAutomation: {fixtures.AutomationMeridianWelcomeName}}, usedBy(refused))

	require.NoError(t, meridian.Automation().Archive(t.Context(), fixtures.AutomationMeridianWelcomeID))

	assert.NoError(t, del())
}
