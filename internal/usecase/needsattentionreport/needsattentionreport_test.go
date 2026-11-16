package needsattentionreport_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/holdreason"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/needsattentionreport"
)

// itemsOf is what needs attention in the Workspace, as the Instance's report reads it.
func itemsOf(t *testing.T, env *testhelper.TestEnv, workspaceID int64) []needsattentionreport.Item {
	t.Helper()
	items, err := testhelper.Module[*needsattentionreport.UseCase](env).Items(t.Context(), env.DB.Scoped(workspaceID))
	require.NoError(t, err)
	return items
}

// kinds is the kind of every item, in the order the report gives them.
func kinds(items []needsattentionreport.Item) []needsattentionreport.Kind {
	return lo.Map(items, func(i needsattentionreport.Item, _ int) needsattentionreport.Kind { return i.Kind })
}

// itemAbout is the item of the kind about the subject id, and whether there is one.
func itemAbout(items []needsattentionreport.Item, kind needsattentionreport.Kind, id int64) (needsattentionreport.Item, bool) {
	return lo.Find(items, func(i needsattentionreport.Item) bool {
		return i.Kind == kind && i.Subject != nil && i.Subject.ID == id
	})
}

// itemOfKind is the item of the kind, and whether there is one.
func itemOfKind(items []needsattentionreport.Item, kind needsattentionreport.Kind) (needsattentionreport.Item, bool) {
	return lo.Find(items, func(i needsattentionreport.Item) bool { return i.Kind == kind })
}

func TestAHealthyWorkspaceNeedsNoAttention(t *testing.T) {
	env := testhelper.Setup(t)

	items := itemsOf(t, env, fixtures.VandelayID)

	assert.Empty(t, items)
}

// Fixtures: Acme's Sending domain lost.acme.com lost its DKIM record, and two of its Webhook
// endpoints disabled themselves.
func TestWhatStopsMailComesBeforeWhatFailed(t *testing.T) {
	env := testhelper.Setup(t)

	items := itemsOf(t, env, fixtures.AcmeID)

	assert.Equal(t, []needsattentionreport.Kind{
		needsattentionreport.KindSendingDomainVerificationLost,
		needsattentionreport.KindWebhookDisabled,
		needsattentionreport.KindWebhookDisabled,
	}, kinds(items))
	lost := items[0]
	assert.Equal(t, needsattentionreport.SeverityBlocked, lost.Severity)
	assert.Equal(t, &needsattentionreport.Subject{ID: fixtures.SendingDomainLostID, Name: fixtures.SendingDomainLostDomain}, lost.Subject)
	assert.NotNil(t, lost.Since, "the moment the record was found gone")
	disabled, ok := itemAbout(items, needsattentionreport.KindWebhookDisabled, fixtures.WebhookCodebasicsDisabledID)
	require.True(t, ok)
	assert.Equal(t, needsattentionreport.SeverityNotice, disabled.Severity)
	assert.NotNil(t, disabled.Since, "the moment it disabled itself")
}

// Fixtures: Meridian's scheduled Broadcast from news.meridian.test is held for that unverified domain,
// its sending one for a Workspace-level reason, and its failed one keeps the reason it was held for.
func TestAHeldBroadcastSaysWhyButAWorkspaceLevelHoldIsLeftToTheLayout(t *testing.T) {
	env := testhelper.Setup(t)

	items := itemsOf(t, env, fixtures.MeridianID)

	held, ok := itemAbout(items, needsattentionreport.KindBroadcastHeld, fixtures.BroadcastMeridianFromNewsID)
	require.True(t, ok)
	assert.Equal(t, needsattentionreport.SeverityBlocked, held.Severity)
	assert.Equal(t, new(holdreason.UnverifiedDomain), held.HoldReason)
	_, suspended := itemAbout(items, needsattentionreport.KindBroadcastHeld, fixtures.BroadcastMeridianSendingID)
	assert.False(t, suspended, "a suspension is the layout's banner")
	_, failed := itemAbout(items, needsattentionreport.KindBroadcastHeld, fixtures.BroadcastMeridianFailedFromMailID)
	assert.False(t, failed, "a failed Broadcast keeps its reason but is no longer held")
}

// Fixtures: Meridian's stopped relay was stopped by its provider; its default is on probation and in
// its provider's sandbox.
func TestTheProviderStandingOfAnIntegrationIsAnItem(t *testing.T) {
	env := testhelper.Setup(t)

	items := itemsOf(t, env, fixtures.MeridianID)

	stopped, ok := itemAbout(items, needsattentionreport.KindIntegrationProviderStopped, fixtures.IntegrationMeridianStoppedID)
	require.True(t, ok)
	assert.Equal(t, needsattentionreport.SeverityBlocked, stopped.Severity)
	probation, ok := itemAbout(items, needsattentionreport.KindIntegrationProviderProbation, fixtures.IntegrationMeridianDefaultID)
	require.True(t, ok)
	assert.Equal(t, needsattentionreport.SeverityDegraded, probation.Severity)
	sandbox, ok := itemAbout(items, needsattentionreport.KindIntegrationProviderSandbox, fixtures.IntegrationMeridianDefaultID)
	require.True(t, ok)
	assert.Equal(t, needsattentionreport.SeverityDegraded, sandbox.Severity)
}

func TestAnIntegrationWhoseCredentialsTheProviderRefusedIsABlockedItemSinceTheRefusal(t *testing.T) {
	env := testhelper.Setup(t)
	refusedAt := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	require.NoError(t, env.DB.Scoped(fixtures.AcmeID).Integration().UpdateOneID(fixtures.IntegrationAcmeDefaultID).
		SetProviderRefusedAt(refusedAt).Exec(t.Context()))

	items := itemsOf(t, env, fixtures.AcmeID)

	refused, ok := itemAbout(items, needsattentionreport.KindIntegrationProviderRefused, fixtures.IntegrationAcmeDefaultID)
	require.True(t, ok)
	assert.Equal(t, needsattentionreport.SeverityBlocked, refused.Severity)
	require.NotNil(t, refused.Since)
	assert.True(t, refusedAt.Equal(*refused.Since), "since is when the provider refused, got %v", *refused.Since)
}

// Fixtures: Acme's default Integration sent messages over the last 24 hours.
func TestASpentProviderLimitIsAnItemAndALimitWithRoomIsNot(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, acme.Integration().UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerDay(1000).Exec(t.Context()))
	roomy := itemsOf(t, env, fixtures.AcmeID)
	require.NoError(t, acme.Integration().UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerDay(1).Exec(t.Context()))

	spent := itemsOf(t, env, fixtures.AcmeID)

	_, before := itemAbout(roomy, needsattentionreport.KindProviderLimitSpent, fixtures.IntegrationAcmeDefaultID)
	after, ok := itemAbout(spent, needsattentionreport.KindProviderLimitSpent, fixtures.IntegrationAcmeDefaultID)
	assert.False(t, before)
	require.True(t, ok)
	assert.Equal(t, needsattentionreport.SeverityDegraded, after.Severity)
}

// Fixtures: Acme sent messages over the last 24 hours; the harness runs in the saas Mode, which
// has Send caps.
func TestASpentSendCapIsAnItemOfTheWorkspaceItself(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(1).ExecX(t.Context())

	items := itemsOf(t, env, fixtures.AcmeID)

	capped, ok := itemOfKind(items, needsattentionreport.KindSendCapSpent)
	require.True(t, ok)
	assert.Equal(t, needsattentionreport.SeverityDegraded, capped.Severity)
	assert.Nil(t, capped.Subject, "the Send cap is the Workspace's own")
}

// Fixtures: two Enrollments of Meridian's welcome failed within the last hours, a third eight days ago.
func TestTheRunsOfAnAutomationThatFailedThisWeekAreOneItemWithTheirCount(t *testing.T) {
	env := testhelper.Setup(t)

	items := itemsOf(t, env, fixtures.MeridianID)

	failed, ok := itemAbout(items, needsattentionreport.KindEnrollmentsFailed, fixtures.AutomationMeridianWelcomeID)
	require.True(t, ok)
	assert.Equal(t, 2, failed.Count, "the run of eight days ago is no longer an item")
	assert.Equal(t, needsattentionreport.SeverityNotice, failed.Severity)
	assert.NotNil(t, failed.Since, "when the first of them failed")
}

// What asks for attention leaves archived rows out (ADR 0099), as it does an archived Broadcast.
// Fixtures: an Enrollment of Meridian's archived Automation failed within the last hour.
func TestTheFailedRunsOfAnArchivedAutomationAreNoItem(t *testing.T) {
	env := testhelper.Setup(t)

	items := itemsOf(t, env, fixtures.MeridianID)

	_, failed := itemAbout(items, needsattentionreport.KindEnrollmentsFailed, fixtures.AutomationMeridianArchivedID)
	assert.False(t, failed)
}

func TestABroadcastThatFailedThisWeekIsAnItemSinceItFailed(t *testing.T) {
	env := testhelper.Setup(t)
	failedAt := time.Now().AddDate(0, 0, -2).Truncate(time.Microsecond)
	require.NoError(t, env.DB.Scoped(fixtures.AcmeID).Broadcast().UpdateOneID(fixtures.BroadcastFailedID).SetFailedAt(failedAt).Exec(t.Context()))

	items := itemsOf(t, env, fixtures.AcmeID)

	failed, ok := itemAbout(items, needsattentionreport.KindBroadcastFailed, fixtures.BroadcastFailedID)
	require.True(t, ok)
	assert.Equal(t, needsattentionreport.SeverityNotice, failed.Severity)
	require.NotNil(t, failed.Since)
	assert.True(t, failedAt.Equal(*failed.Since), "since is when it failed, got %v", *failed.Since)
}

func TestABroadcastThatFailedLongAgoIsNoItemAfterAnEdit(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, acme.Broadcast().UpdateOneID(fixtures.BroadcastFailedID).
		SetFailedAt(time.Now().AddDate(0, 0, -8)).Exec(t.Context()))
	require.NoError(t, acme.Broadcast().UpdateOneID(fixtures.BroadcastFailedID).SetName("Renamed today").Exec(t.Context()))

	items := itemsOf(t, env, fixtures.AcmeID)

	_, failed := itemAbout(items, needsattentionreport.KindBroadcastFailed, fixtures.BroadcastFailedID)
	assert.False(t, failed, "a rename does not bring back a failure of eight days ago")
}

// A verified Sending domain whose last DMARC lookup found no record is not bulk-ready (ADR 0012): a
// notice that names it, until a lookup finds the record. Fixtures: mail.acme.com is verified and its
// last lookup found a record.
func TestAVerifiedDomainWithoutADMARCRecordIsANoticeUntilOneIsFound(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, acme.SendingDomain().UpdateOneID(fixtures.SendingDomainVerifiedID).ClearDmarcFoundAt().Exec(t.Context()))
	missing := itemsOf(t, env, fixtures.AcmeID)
	require.NoError(t, acme.SendingDomain().UpdateOneID(fixtures.SendingDomainVerifiedID).SetDmarcFoundAt(time.Now()).Exec(t.Context()))

	found := itemsOf(t, env, fixtures.AcmeID)

	notice, ok := itemAbout(missing, needsattentionreport.KindSendingDomainNotBulkReady, fixtures.SendingDomainVerifiedID)
	require.True(t, ok)
	assert.Equal(t, needsattentionreport.SeverityNotice, notice.Severity)
	assert.Equal(t, fixtures.SendingDomainVerifiedDomain, notice.Subject.Name)
	assert.Nil(t, notice.Since)
	_, still := itemAbout(found, needsattentionreport.KindSendingDomainNotBulkReady, fixtures.SendingDomainVerifiedID)
	assert.False(t, still)
}

// Only a verified, kept domain whose DMARC was looked up is judged: a domain whose DMARC lookup
// never ran or never succeeded (a DKIM check alone says nothing of DMARC), an unverified one (a
// lost verification is an item of its own) and an archived one are not notices.
// Fixtures: none of Meridian's domains had a DMARC record found. mail.meridian.test is verified and its
// DKIM checked, but its DMARC never looked up; news.meridian.test is looked up but unverified; the
// archived old.meridian.test is verified and looked up.
func TestOnlyALookedUpVerifiedKeptDomainIsJudgedBulkReady(t *testing.T) {
	env := testhelper.Setup(t)

	items := itemsOf(t, env, fixtures.MeridianID)

	assert.NotContains(t, kinds(items), needsattentionreport.KindSendingDomainNotBulkReady)
}

// Acme has email Integrations: while its postal address is empty its marketing email cannot be
// armed (ADR 0156).
func TestAWorkspaceWithAnEmailIntegrationAndNoPostalAddressNeedsAttention(t *testing.T) {
	env := testhelper.Setup(t)
	withAddress := kinds(itemsOf(t, env, fixtures.AcmeID))
	env.ClearPostalAddress(t, fixtures.AcmeID)

	items := itemsOf(t, env, fixtures.AcmeID)

	assert.NotContains(t, withAddress, needsattentionreport.KindPostalAddressMissing)
	missing, ok := itemOfKind(items, needsattentionreport.KindPostalAddressMissing)
	require.True(t, ok)
	assert.Equal(t, needsattentionreport.SeverityNotice, missing.Severity)
	assert.Nil(t, missing.Subject, "the address is the Workspace's own")
}
