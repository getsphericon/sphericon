package site_test

import (
	"context"
	"fmt"
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
	del := func() siteapi.SiteIntegrationsDeleteRes {
		got, err := c.SiteIntegrationsDelete(ctx, siteapi.SiteIntegrationsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.IntegrationAcmeDefaultID)})
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
	_, err = acme.Automation().Update().SetStatus(automation.StatusInactive).Save(ctx)
	require.NoError(t, err)
	finishEnrollments(t, acme)

	assert.IsType(t, &siteapi.SiteIntegrationsDeleteNoContent{}, del())
}

// An Integration that is not the default is used only by a Broadcast that names it.
func TestSiteIntegrationArchiveNamedByALiveBroadcast(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	params := siteapi.SiteIntegrationsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.IntegrationAcmeSesID)}

	_, err := acme.Broadcast().UpdateOneID(fixtures.BroadcastScheduledID).SetIntegrationID(fixtures.IntegrationAcmeSesID).Save(ctx)
	require.NoError(t, err)
	got, err := c.SiteIntegrationsDelete(ctx, params)
	require.NoError(t, err)
	refused, ok := got.(*siteapi.InUseProblemDetails)
	require.True(t, ok)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindBroadcast: {fixtures.BroadcastScheduledName}}, usedBy(refused))

	_, err = acme.Broadcast().UpdateOneID(fixtures.BroadcastScheduledID).SetStatus(broadcast.StatusSent).SetSentAt(time.Now()).Save(ctx)
	require.NoError(t, err)
	got, err = c.SiteIntegrationsDelete(ctx, params)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteIntegrationsDeleteNoContent{}, got)
}

// A Sending domain is used through the domain of a From address, not an id: a scheduled Broadcast
// from news.acme.com blocks that domain, and a failed one from mail.acme.com does not block this.
func TestSiteSendingDomainDeleteRefusedThroughAFromAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	del := func(id int64) siteapi.SiteSendingDomainsDeleteRes {
		return deleteSendingDomain(t, c, id)
	}
	_, err := acme.Broadcast().UpdateOneID(fixtures.BroadcastScheduledID).SetFromEmail("hello@" + fixtures.SendingDomainUnverifiedDomain).Save(ctx)
	require.NoError(t, err)
	_, err = acme.Broadcast().UpdateOneID(fixtures.BroadcastFailedID).SetFromEmail("hello@" + fixtures.SendingDomainVerifiedDomain).Save(ctx)
	require.NoError(t, err)

	refused, ok := del(fixtures.SendingDomainUnverifiedID).(*siteapi.InUseProblemDetails)
	require.True(t, ok)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindBroadcast: {fixtures.BroadcastScheduledName}}, usedBy(refused))

	assert.IsType(t, &siteapi.SiteSendingDomainsDeleteNoContent{}, del(fixtures.SendingDomainVerifiedID))
}

// A Broadcast with no From and an active Automation send from the default Integration's configured
// From (hello@codebasics.dev), so that domain is refused until nothing live sends from it.
func TestSiteSendingDomainDeleteRefusedThroughTheConfiguredFrom(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	del := func() siteapi.SiteSendingDomainsDeleteRes {
		return deleteSendingDomain(t, c, fixtures.SendingDomainCodebasicsID)
	}

	refused, ok := del().(*siteapi.InUseProblemDetails)
	require.True(t, ok)
	users := usedBy(refused)
	assert.ElementsMatch(t, []string{fixtures.BroadcastScheduledName, fixtures.BroadcastSendingName}, users[siteapi.UsedByKindBroadcast])
	assert.Contains(t, users[siteapi.UsedByKindAutomation], fixtures.AutomationWelcomeSeriesName)

	_, err := acme.Broadcast().Update().Where(broadcast.StatusIn(broadcast.StatusScheduled, broadcast.StatusSending)).SetStatus(broadcast.StatusSent).SetSentAt(time.Now()).Save(ctx)
	require.NoError(t, err)
	_, err = acme.Automation().Update().SetStatus(automation.StatusInactive).Save(ctx)
	require.NoError(t, err)
	finishEnrollments(t, acme)
	assert.IsType(t, &siteapi.SiteSendingDomainsDeleteNoContent{}, del())
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

	_, err = c.SiteSendingDomainsDelete(ctx, siteapi.SiteSendingDomainsDeleteParams{
		WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.SendingDomainCodebasicsID),
	})

	require.Error(t, err, "the unreadable definition must fail the removal check")
	got, err := c.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{
		WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.SendingDomainCodebasicsID),
	})
	require.NoError(t, err)
	kept, ok := got.(*siteapi.SiteSendingDomainResource)
	require.Truef(t, ok, "got %T", got)
	deleted := kept.DiscardedAt.IsSet()
	assert.False(t, deleted)
}

// A draft's email does not establish a dependency, nor does the live version of an inactive or
// archived Automation no Enrollment stands on any more.
func TestDraftOnlyAndStoppedSendingDoNotPreventIntegrationRemoval(t *testing.T) {
	// Each email Step names the Integration it sends through (ADR 0081).
	mailgunEmail := fmt.Sprintf(`[{"type":"email","integrationId":%d,"subject":"Hello"}]`, fixtures.IntegrationAcmeMailgunID)
	for _, tc := range []struct {
		name     string
		steps    string
		status   automation.Status
		archived *time.Time
	}{
		{name: "draft only", steps: `[{"type":"wait","seconds":60}]`, status: automation.StatusActive},
		{name: "HTTP and Tags send no email", steps: `[{"type":"http","body":"email"},{"type":"apply_tag","tag":"email"}]`, status: automation.StatusActive},
		{name: "inactive", steps: mailgunEmail, status: automation.StatusInactive},
		{name: "archived", steps: mailgunEmail, status: automation.StatusActive, archived: new(time.Now())},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			s := env.DB.Scoped(fixtures.AcmeID)
			ctx := t.Context()
			stopMailgunSender(t, s)
			err := s.AutomationVersion().UpdateOneID(fixtures.AutomationWelcomeSeriesVersionID).
				SetSteps(tc.steps).Exec(ctx)
			require.NoError(t, err)
			err = s.AutomationVersion().UpdateOneID(fixtures.AutomationWelcomeSeriesDraftVersionID).
				SetSteps(mailgunEmail).Exec(ctx)
			require.NoError(t, err)
			err = s.Automation().UpdateOneID(fixtures.AutomationWelcomeSeriesID).
				SetStatus(tc.status).SetNillableDiscardedAt(tc.archived).Exec(ctx)
			require.NoError(t, err)
			// An Enrollment still standing on the version keeps it sending
			// (TestSiteIntegrationIsUsedByTheVersionAnUnfinishedEnrollmentStandsOn).
			finishEnrollments(t, s)

			require.NoError(t, env.Integrations.Delete(ctx, s, fixtures.IntegrationAcmeMailgunID))

			deleted, err := env.Integrations.Get(ctx, s, fixtures.IntegrationAcmeMailgunID)
			require.NoError(t, err)
			assert.NotNil(t, deleted.DiscardedAt)
		})
	}
}

// stopMailgunSender stops the fixture Automation whose second email step sends through the
// Mailgun Integration, for a test about who else uses it.
func stopMailgunSender(t *testing.T, s *ent.Scoped) {
	t.Helper()
	err := s.Automation().UpdateOneID(fixtures.AutomationJobsEmailsThenTagID).
		SetStatus(automation.StatusInactive).Exec(t.Context())
	require.NoError(t, err)
}

// A Broadcast that names no Integration sends through its own Channel's default, never another
// Channel's (ADR 0099): a scheduled Telegram one blocks deleting the Telegram default and is no
// user of the email default.
func TestSiteTelegramBroadcastUsesTheTelegramDefaultOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	err := acme.Integration().UpdateOneID(fixtures.IntegrationAcmeTelegramID).SetIsDefault(true).Exec(ctx)
	require.NoError(t, err)
	err = acme.Broadcast().UpdateOneID(fixtures.BroadcastTelegramDraftID).
		SetStatus(broadcast.StatusScheduled).SetScheduledAt(time.Now().Add(time.Hour)).Exec(ctx)
	require.NoError(t, err)

	refused, ok := deleteIntegration(t, c, fixtures.IntegrationAcmeTelegramID).(*siteapi.InUseProblemDetails)
	require.True(t, ok)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindBroadcast: {fixtures.BroadcastTelegramDraftName}}, usedBy(refused))

	refused, ok = deleteIntegration(t, c, fixtures.IntegrationAcmeDefaultID).(*siteapi.InUseProblemDetails)
	require.True(t, ok, "the email default is still used by the email Broadcasts")
	assert.NotContains(t, usedBy(refused)[siteapi.UsedByKindBroadcast], fixtures.BroadcastTelegramDraftName)
}

// A send with no From of its own goes out with the From of the Integration it goes through, so a
// Broadcast and an Automation step through a non-default Integration use that Integration's From
// domain, not the default's (ADR 0099).
func TestSiteSendingDomainIsUsedThroughTheFromOfTheIntegrationASendGoesThrough(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	sealed, err := env.Cipher.Encrypt([]byte(`{"host":"smtp.postmarkapp.example","port":587,"from":"hello@` + fixtures.SendingDomainUnverifiedDomain + `"}`))
	require.NoError(t, err)
	err = acme.Integration().UpdateOneID(fixtures.IntegrationAcmePostmarkID).SetConfigEncrypted(sealed).Exec(ctx)
	require.NoError(t, err)
	err = acme.Broadcast().UpdateOneID(fixtures.BroadcastScheduledID).
		ClearFromEmail().SetIntegrationID(fixtures.IntegrationAcmePostmarkID).Exec(ctx)
	require.NoError(t, err)
	err = acme.AutomationVersion().UpdateOneID(fixtures.AutomationWelcomeSeriesVersionID).
		SetSteps(fmt.Sprintf(`[{"type":"email","integrationId":%d,"subject":"Hello"}]`, fixtures.IntegrationAcmePostmarkID)).Exec(ctx)
	require.NoError(t, err)
	finishEnrollments(t, acme) // the Automation uses the domain through its live version alone

	refused, ok := deleteSendingDomain(t, c, fixtures.SendingDomainUnverifiedID).(*siteapi.InUseProblemDetails)
	require.True(t, ok)
	assert.Equal(t, map[siteapi.UsedByKind][]string{
		siteapi.UsedByKindBroadcast:  {fixtures.BroadcastScheduledName},
		siteapi.UsedByKindAutomation: {fixtures.AutomationWelcomeSeriesName},
	}, usedBy(refused))

	refused, ok = deleteSendingDomain(t, c, fixtures.SendingDomainCodebasicsID).(*siteapi.InUseProblemDetails)
	require.True(t, ok, "the default's domain is still used by the Sending Broadcast")
	users := usedBy(refused)
	assert.NotContains(t, users[siteapi.UsedByKindBroadcast], fixtures.BroadcastScheduledName)
	assert.NotContains(t, users[siteapi.UsedByKindAutomation], fixtures.AutomationWelcomeSeriesName)
}

// An Automation deactivated while Enrollments still stand on a version keeps sending through that
// version's Integrations, so it uses them until the Enrollments finish (ADR 0099).
func TestSiteIntegrationIsUsedByTheVersionAnUnfinishedEnrollmentStandsOn(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	stopMailgunSender(t, acme)
	err := acme.AutomationVersion().UpdateOneID(fixtures.AutomationWelcomeSeriesVersionID).
		SetSteps(fmt.Sprintf(`[{"type":"email","integrationId":%d,"subject":"Hello"}]`, fixtures.IntegrationAcmeMailgunID)).Exec(ctx)
	require.NoError(t, err)
	err = acme.Automation().UpdateOneID(fixtures.AutomationWelcomeSeriesID).
		SetStatus(automation.StatusInactive).Exec(ctx)
	require.NoError(t, err)

	refused, ok := deleteIntegration(t, c, fixtures.IntegrationAcmeMailgunID).(*siteapi.InUseProblemDetails)
	require.True(t, ok)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindAutomation: {fixtures.AutomationWelcomeSeriesName}}, usedBy(refused))

	finishEnrollments(t, acme)
	assert.IsType(t, &siteapi.SiteIntegrationsDeleteNoContent{}, deleteIntegration(t, c, fixtures.IntegrationAcmeMailgunID))
}

// finishEnrollments completes the Workspace's unfinished Enrollments: no version sends any more
// for want of an Enrollment standing on it.
func finishEnrollments(t *testing.T, s *ent.Scoped) {
	t.Helper()
	err := s.AutomationRun().Update().
		Where(automationrun.StatusEQ(automationrun.StatusActive)).
		SetStatus(automationrun.StatusCompleted).SetEndedAt(time.Now()).Exec(t.Context())
	require.NoError(t, err)
}
