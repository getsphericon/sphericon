package site_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/broadcast"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func usedBy(refused *siteapi.InUseProblemDetails) map[siteapi.UsedByKind][]string {
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

	byRule, err := c.SiteTagsDelete(ctx, siteapi.SiteTagsDeleteParams{WorkspaceId: ws, ID: idStr(fixtures.TagNewsletterID)})
	require.NoError(t, err)
	refused, ok := byRule.(*siteapi.InUseProblemDetails)
	require.Truef(t, ok, "got %T", byRule)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindSegment: {fixtures.SegmentNewsletterReadersName}}, usedBy(refused))

	byStep, err := c.SiteTagsDelete(ctx, siteapi.SiteTagsDeleteParams{WorkspaceId: ws, ID: idStr(fixtures.TagVipID)})
	require.NoError(t, err)
	refused, ok = byStep.(*siteapi.InUseProblemDetails)
	require.Truef(t, ok, "got %T", byStep)
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

	got, err := c.SiteTagsDelete(ctx, siteapi.SiteTagsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.TagUnusedID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsDeleteNoContent{}, got)

	again, err := c.SiteTagsDelete(ctx, siteapi.SiteTagsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.TagUnusedID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsDeleteNotFound{}, again)
}

// The default Integration is used by a scheduled or sending Broadcast that names none, and by an
// active Automation that sends email; once only sent or failed Broadcasts and no such Automation
// remain, it is archived.
func TestSiteDefaultIntegrationArchiveRefusedWhileLiveWorkUsesIt(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	del := func() siteapi.SiteIntegrationsArchiveRes {
		got, err := c.SiteIntegrationsArchive(ctx, siteapi.SiteIntegrationsArchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.IntegrationAcmeDefaultID)})
		require.NoError(t, err)
		return got
	}

	refused, ok := del().(*siteapi.InUseProblemDetails)
	require.True(t, ok)
	users := usedBy(refused)
	assert.Contains(t, users[siteapi.UsedByKindBroadcast], fixtures.BroadcastScheduledName)
	assert.Contains(t, users[siteapi.UsedByKindBroadcast], fixtures.BroadcastSendingName)
	assert.Contains(t, users[siteapi.UsedByKindAutomation], fixtures.AutomationWelcomeSeriesName)
	assert.EqualValues(t, http.StatusConflict, refused.Status.Or(0))

	// Only history uses it now: finished Broadcasts, and no active Automation.
	_, err := acme.Broadcast().Update().Where(broadcast.StatusIn(broadcast.StatusScheduled, broadcast.StatusSending)).SetStatus(broadcast.StatusSent).SetSentAt(time.Now()).Save(ctx)
	require.NoError(t, err)
	_, err = acme.Automation().Update().SetStatus(automation.StatusDraft).Save(ctx)
	require.NoError(t, err)

	assert.IsType(t, &siteapi.SiteIntegrationResource{}, del())
}

// An Integration that is not the default is used only by a Broadcast that names it.
func TestSiteIntegrationArchiveNamedByALiveBroadcast(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	params := siteapi.SiteIntegrationsArchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.IntegrationAcmeSesID)}

	_, err := acme.Broadcast().UpdateOneID(fixtures.BroadcastScheduledID).SetIntegrationID(fixtures.IntegrationAcmeSesID).Save(ctx)
	require.NoError(t, err)
	got, err := c.SiteIntegrationsArchive(ctx, params)
	require.NoError(t, err)
	refused, ok := got.(*siteapi.InUseProblemDetails)
	require.True(t, ok)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindBroadcast: {fixtures.BroadcastScheduledName}}, usedBy(refused))

	_, err = acme.Broadcast().UpdateOneID(fixtures.BroadcastScheduledID).SetStatus(broadcast.StatusSent).SetSentAt(time.Now()).Save(ctx)
	require.NoError(t, err)
	got, err = c.SiteIntegrationsArchive(ctx, params)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteIntegrationResource{}, got)
}

// A Sending domain is used through the domain of a From address, not an id: a scheduled Broadcast
// from news.acme.com blocks that domain, and a failed one from mail.acme.com does not block this.
func TestSiteSendingDomainArchiveRefusedThroughAFromAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	del := func(id int64) siteapi.SiteSendingDomainsArchiveRes {
		return archiveSendingDomain(t, c, id)
	}
	_, err := acme.Broadcast().UpdateOneID(fixtures.BroadcastScheduledID).SetFromEmail("hello@" + fixtures.SendingDomainUnverifiedDomain).Save(ctx)
	require.NoError(t, err)
	_, err = acme.Broadcast().UpdateOneID(fixtures.BroadcastFailedID).SetFromEmail("hello@" + fixtures.SendingDomainVerifiedDomain).Save(ctx)
	require.NoError(t, err)

	refused, ok := del(fixtures.SendingDomainUnverifiedID).(*siteapi.InUseProblemDetails)
	require.True(t, ok)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindBroadcast: {fixtures.BroadcastScheduledName}}, usedBy(refused))

	assert.IsType(t, &siteapi.SiteSendingDomainResource{}, del(fixtures.SendingDomainVerifiedID))
}

// A Broadcast with no From and an active Automation send from the default Integration's configured
// From (hello@codebasics.dev), so that domain is refused until nothing live sends from it.
func TestSiteSendingDomainArchiveRefusedThroughTheConfiguredFrom(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	del := func() siteapi.SiteSendingDomainsArchiveRes {
		return archiveSendingDomain(t, c, fixtures.SendingDomainCodebasicsID)
	}

	refused, ok := del().(*siteapi.InUseProblemDetails)
	require.True(t, ok)
	users := usedBy(refused)
	assert.ElementsMatch(t, []string{fixtures.BroadcastScheduledName, fixtures.BroadcastSendingName}, users[siteapi.UsedByKindBroadcast])
	assert.Contains(t, users[siteapi.UsedByKindAutomation], fixtures.AutomationWelcomeSeriesName)

	_, err := acme.Broadcast().Update().Where(broadcast.StatusIn(broadcast.StatusScheduled, broadcast.StatusSending)).SetStatus(broadcast.StatusSent).SetSentAt(time.Now()).Save(ctx)
	require.NoError(t, err)
	_, err = acme.Automation().Update().SetStatus(automation.StatusDraft).Save(ctx)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainResource{}, del())
}
