package site_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/broadcast"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func usedBy(refused siteapi.ProblemDetails) map[siteapi.UsedByKind][]string {
	out := map[siteapi.UsedByKind][]string{}
	for _, u := range refused.UsedBy {
		out[u.Kind] = append(out[u.Kind], u.Name)
	}
	return out
}

// A Tag that an Automation step or a Segment rule (at any depth) names cannot be deleted: the
// 409 names both kinds of user, and the Tag stays (ADR 0049).
func TestSiteTagDeleteRefusedWhileARuleOrAStepNamesIt(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)

	err := c.SiteTagsDelete(ctx, siteapi.SiteTagsDeleteParams{WorkspaceId: ws, ID: idStr(fixtures.TagNewsletterID)})
	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindSegment: {fixtures.SegmentNewsletterReadersName}}, usedBy(refused))

	err = c.SiteTagsDelete(ctx, siteapi.SiteTagsDeleteParams{WorkspaceId: ws, ID: idStr(fixtures.TagVipID)})
	refused = testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Equal(t, map[siteapi.UsedByKind][]string{
		siteapi.UsedByKindAutomation: {fixtures.AutomationTagOnEngagementName},
		// An active Rule names the Tag in its Condition (RuleVipOnly) or in an Action (RuleMultiTrigger).
		siteapi.UsedByKindRule: {fixtures.RuleVipOnlyName, fixtures.RuleMultiTriggerName},
	}, usedBy(refused))

	_, err = env.DB.Scoped(fixtures.AcmeID).Tag().Get(ctx, fixtures.TagVipID)
	require.NoError(t, err)
}

// A Tag nothing names is deleted.
func TestSiteTagDeleteUnusedTag(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	err := c.SiteTagsDelete(ctx, siteapi.SiteTagsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.TagUnusedID)})
	require.NoError(t, err)

	err = c.SiteTagsDelete(ctx, siteapi.SiteTagsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.TagUnusedID)})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}

// The default Integration is used by a scheduled or sending Broadcast that names none, and by an
// active Automation that sends email; once only sent Broadcasts and no such Automation remain, it is
// archived.
func TestSiteDefaultIntegrationArchiveRefusedWhileLiveWorkUsesIt(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MeridianOwnerMonaEmail)
	ctx := context.Background()
	meridian := env.DB.Scoped(fixtures.MeridianID)
	del := func() error {
		return deleteIntegrationOf(t, c, fixtures.MeridianPublicID, fixtures.IntegrationMeridianDefaultID)
	}

	refused := testhelper.SiteRefusal(t, del(), http.StatusConflict)
	assert.Equal(t, map[siteapi.UsedByKind][]string{
		siteapi.UsedByKindBroadcast:  {fixtures.BroadcastMeridianScheduledName, fixtures.BroadcastMeridianSendingName},
		siteapi.UsedByKindAutomation: {fixtures.AutomationMeridianWelcomeName},
	}, usedBy(refused))
	assert.EqualValues(t, http.StatusConflict, refused.Status.Or(0))

	// Only history uses it now: the Broadcasts are sent and the Automation is inactive.
	sendMeridianBroadcasts(t, meridian)
	require.NoError(t, meridian.Automation().UpdateOneID(fixtures.AutomationMeridianWelcomeID).SetStatus(automation.StatusInactive).Exec(ctx))

	require.NoError(t, del())
}

// sendMeridianBroadcasts finishes the scheduled and the sending Broadcast of Meridian's default: they
// become history.
func sendMeridianBroadcasts(t *testing.T, s *ent.Scoped) {
	t.Helper()
	for _, id := range []int64{fixtures.BroadcastMeridianScheduledID, fixtures.BroadcastMeridianSendingID} {
		require.NoError(t, s.Broadcast().UpdateOneID(id).SetStatus(broadcast.StatusSent).SetSentAt(time.Now()).Exec(t.Context()))
	}
}

// An Integration that is not the default is used only by a Broadcast that names it.
func TestSiteIntegrationArchiveNamedByALiveBroadcast(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MeridianOwnerMonaEmail)
	ctx := context.Background()
	meridian := env.DB.Scoped(fixtures.MeridianID)

	refused := testhelper.SiteRefusal(t, deleteIntegrationOf(t, c, fixtures.MeridianPublicID, fixtures.IntegrationMeridianNamedID), http.StatusConflict)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindBroadcast: {fixtures.BroadcastMeridianNamedName}}, usedBy(refused))

	_, err := meridian.Broadcast().UpdateOneID(fixtures.BroadcastMeridianNamedID).SetStatus(broadcast.StatusSent).SetSentAt(time.Now()).Save(ctx)
	require.NoError(t, err)
	require.NoError(t, deleteIntegrationOf(t, c, fixtures.MeridianPublicID, fixtures.IntegrationMeridianNamedID))
}

// A Sending domain is used through the domain of a From address, not an id: a scheduled Broadcast
// from news.meridian.test blocks that domain, and a failed one from mail.meridian.test does not block
// this.
func TestSiteSendingDomainDeleteRefusedThroughAFromAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MeridianOwnerMonaEmail)

	refused := testhelper.SiteRefusal(t, deleteSendingDomainOf(t, c, fixtures.MeridianPublicID, fixtures.SendingDomainMeridianNewsID), http.StatusConflict)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindBroadcast: {fixtures.BroadcastMeridianFromNewsName}}, usedBy(refused))

	require.NoError(t, deleteSendingDomainOf(t, c, fixtures.MeridianPublicID, fixtures.SendingDomainMeridianMailID))
}

// A Broadcast with no From and an active Automation send from the default Integration's configured
// From (hello@meridian.test), so that domain is refused until nothing live sends from it.
func TestSiteSendingDomainDeleteRefusedThroughTheConfiguredFrom(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MeridianOwnerMonaEmail)
	ctx := context.Background()
	meridian := env.DB.Scoped(fixtures.MeridianID)
	del := func() error {
		return deleteSendingDomainOf(t, c, fixtures.MeridianPublicID, fixtures.SendingDomainMeridianID)
	}

	refused := testhelper.SiteRefusal(t, del(), http.StatusConflict)
	assert.Equal(t, map[siteapi.UsedByKind][]string{
		siteapi.UsedByKindBroadcast:  {fixtures.BroadcastMeridianScheduledName, fixtures.BroadcastMeridianSendingName},
		siteapi.UsedByKindAutomation: {fixtures.AutomationMeridianWelcomeName},
	}, usedBy(refused))

	sendMeridianBroadcasts(t, meridian)
	require.NoError(t, meridian.Automation().UpdateOneID(fixtures.AutomationMeridianWelcomeID).SetStatus(automation.StatusInactive).Exec(ctx))
	require.NoError(t, del())
}

func TestDamagedStoredStepsDoNotPermitIntegrationRemoval(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ctx := t.Context()
	err := s.AutomationVersion().UpdateOneID(fixtures.AutomationHoldDemoVersionID).
		SetSteps(`[{"type":"wait"},`).Exec(ctx)
	require.NoError(t, err)

	err = env.Integrations.Delete(ctx, s, fixtures.IntegrationAcmeMailgunID)

	require.Error(t, err, "an unreadable definition cannot prove the Integration unused")
	var used *inuse.Error
	assert.NotErrorAs(t, err, &used, "propagate the read failure, rather than claiming known use")
	kept, err := env.Integrations.Get(ctx, s, fixtures.IntegrationAcmeMailgunID)
	require.NoError(t, err)
	assert.Nil(t, kept.DiscardedAt)
}

func TestDamagedStoredStepsDoNotPermitSendingDomainRemoval(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ctx := t.Context()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	err := s.AutomationVersion().UpdateOneID(fixtures.AutomationHoldDemoVersionID).
		SetSteps(`[{"type":"email"},`).Exec(ctx)
	require.NoError(t, err)

	err = c.SiteSendingDomainsDelete(ctx, siteapi.SiteSendingDomainsDeleteParams{
		WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.SendingDomainCodebasicsID),
	})

	require.Error(t, err, "the unreadable definition must fail the removal check")
	kept, err := c.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{
		WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.SendingDomainCodebasicsID),
	})
	require.NoError(t, err)
	deleted := kept.DiscardedAt.IsSet()
	assert.False(t, deleted)
}

// A draft's email does not establish a dependency, nor does the live version of an inactive or
// archived Automation no Enrollment stands on any more. Each Automation sends its email steps
// through an Integration of its own, which nothing else uses.
func TestDraftOnlyAndStoppedSendingDoNotPreventIntegrationRemoval(t *testing.T) {
	for _, tc := range []struct {
		name        string
		integration int64
	}{
		{name: "draft only", integration: fixtures.IntegrationMeridianDraftOnlyID},
		{name: "HTTP and Tags send no email", integration: fixtures.IntegrationMeridianNoEmailID},
		{name: "inactive", integration: fixtures.IntegrationMeridianInactiveID},
		{name: "archived", integration: fixtures.IntegrationMeridianArchivedID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			s := env.DB.Scoped(fixtures.MeridianID)
			ctx := t.Context()

			require.NoError(t, env.Integrations.Delete(ctx, s, tc.integration))

			deleted, err := env.Integrations.Get(ctx, s, tc.integration)
			require.NoError(t, err)
			assert.NotNil(t, deleted.DiscardedAt)
		})
	}
}

// A Broadcast that names no Integration sends through its own Channel's default, never another
// Channel's (ADR 0099): a scheduled Telegram one blocks deleting the Telegram default and is no
// user of the email default.
func TestSiteTelegramBroadcastUsesTheTelegramDefaultOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MeridianOwnerMonaEmail)

	refused := testhelper.SiteRefusal(t, deleteIntegrationOf(t, c, fixtures.MeridianPublicID, fixtures.IntegrationMeridianTelegramID), http.StatusConflict)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindBroadcast: {fixtures.BroadcastMeridianTelegramName}}, usedBy(refused))

	refused = testhelper.SiteRefusal(t, deleteIntegrationOf(t, c, fixtures.MeridianPublicID, fixtures.IntegrationMeridianDefaultID), http.StatusConflict,
		"the email default is still used by the email Broadcasts")
	assert.NotContains(t, usedBy(refused)[siteapi.UsedByKindBroadcast], fixtures.BroadcastMeridianTelegramName)
}

// A send with no From of its own goes out with the From of the Integration it goes through, so a
// Broadcast and an Automation step through a non-default Integration use that Integration's From
// domain, not the default's (ADR 0099).
func TestSiteSendingDomainIsUsedThroughTheFromOfTheIntegrationASendGoesThrough(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MeridianOwnerMonaEmail)

	refused := testhelper.SiteRefusal(t, deleteSendingDomainOf(t, c, fixtures.MeridianPublicID, fixtures.SendingDomainMeridianRelayID), http.StatusConflict)
	assert.Equal(t, map[siteapi.UsedByKind][]string{
		siteapi.UsedByKindBroadcast:  {fixtures.BroadcastMeridianViaRelayName},
		siteapi.UsedByKindAutomation: {fixtures.AutomationMeridianViaRelayName},
	}, usedBy(refused))

	refused = testhelper.SiteRefusal(t, deleteSendingDomainOf(t, c, fixtures.MeridianPublicID, fixtures.SendingDomainMeridianID), http.StatusConflict,
		"the default's domain is still used by the Broadcasts through the default")
	users := usedBy(refused)
	assert.NotContains(t, users[siteapi.UsedByKindBroadcast], fixtures.BroadcastMeridianViaRelayName)
	assert.NotContains(t, users[siteapi.UsedByKindAutomation], fixtures.AutomationMeridianViaRelayName)
}

// An Automation deactivated while Enrollments still stand on a version keeps sending through that
// version's Integrations, so it uses them until the Enrollments finish (ADR 0099).
func TestSiteIntegrationIsUsedByTheVersionAnUnfinishedEnrollmentStandsOn(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MeridianOwnerMonaEmail)
	ctx := t.Context()
	meridian := env.DB.Scoped(fixtures.MeridianID)

	refused := testhelper.SiteRefusal(t, deleteIntegrationOf(t, c, fixtures.MeridianPublicID, fixtures.IntegrationMeridianEnrolledID), http.StatusConflict)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindAutomation: {fixtures.AutomationMeridianEnrolledName}}, usedBy(refused))

	require.NoError(t, meridian.AutomationRun().UpdateOneID(fixtures.AutomationRunMeridianEnrolledID).
		SetStatus(automationrun.StatusCompleted).SetEndedAt(time.Now()).Exec(ctx))
	require.NoError(t, deleteIntegrationOf(t, c, fixtures.MeridianPublicID, fixtures.IntegrationMeridianEnrolledID))
}
