package site_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/holdreason"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// attentionOf is what needs attention in the Workspace, as the member c reads it.
func attentionOf(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId) []siteapi.SiteAttentionItem {
	t.Helper()
	res, err := c.SiteAttentionGet(t.Context(), siteapi.SiteAttentionGetParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	list := res
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

// Fixtures: Meridian's scheduled Broadcast from news.meridian.test is held for that unverified domain,
// its sending one for a Workspace-level reason, and its failed one keeps the reason it was held for.
func TestAHeldBroadcastSaysWhyButAWorkspaceLevelHoldIsLeftToTheLayout(t *testing.T) {
	env := testhelper.Setup(t)

	items := attentionOf(t, env.SiteActor(t, fixtures.MeridianOwnerMonaEmail), fixtures.MeridianPublicID)

	held, ok := itemAbout(items, siteapi.SiteAttentionKindBroadcastHeld, fixtures.BroadcastMeridianFromNewsID)
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteAttentionSeverityBlocked, held.Severity)
	assert.Equal(t, siteapi.HoldReasonUnverifiedDomain, held.HoldReason.Value)
	_, suspended := itemAbout(items, siteapi.SiteAttentionKindBroadcastHeld, fixtures.BroadcastMeridianSendingID)
	assert.False(t, suspended, "a suspension is the layout's banner")
	_, failed := itemAbout(items, siteapi.SiteAttentionKindBroadcastHeld, fixtures.BroadcastMeridianFailedFromMailID)
	assert.False(t, failed, "a failed Broadcast keeps its reason but is no longer held")
}

// Fixtures: Meridian's stopped relay was stopped by its provider; its default is on probation and in
// its provider's sandbox.
func TestTheProviderStandingOfAnIntegrationIsAnItem(t *testing.T) {
	env := testhelper.Setup(t)

	items := attentionOf(t, env.SiteActor(t, fixtures.MeridianOwnerMonaEmail), fixtures.MeridianPublicID)

	stopped, ok := itemAbout(items, siteapi.SiteAttentionKindIntegrationProviderStopped, fixtures.IntegrationMeridianStoppedID)
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteAttentionSeverityBlocked, stopped.Severity)
	probation, ok := itemAbout(items, siteapi.SiteAttentionKindIntegrationProviderProbation, fixtures.IntegrationMeridianDefaultID)
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteAttentionSeverityDegraded, probation.Severity)
	_, sandbox := itemAbout(items, siteapi.SiteAttentionKindIntegrationProviderSandbox, fixtures.IntegrationMeridianDefaultID)
	assert.True(t, sandbox)
}

func TestAnIntegrationWhoseCredentialsTheProviderRefusedIsABlockedItem(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, env.DB.Scoped(fixtures.AcmeID).Integration().UpdateOneID(fixtures.IntegrationAcmeDefaultID).
		SetProviderRefusedAt(time.Now()).Exec(t.Context()))

	items := attentionOf(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID)

	refused, ok := itemAbout(items, siteapi.SiteAttentionKindIntegrationProviderRefused, fixtures.IntegrationAcmeDefaultID)
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteAttentionSeverityBlocked, refused.Severity)
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

// Fixtures: two Enrollments of Meridian's welcome failed within the last hours, a third eight days ago.
func TestTheRunsOfAnAutomationThatFailedThisWeekAreOneItemWithTheirCount(t *testing.T) {
	env := testhelper.Setup(t)

	items := attentionOf(t, env.SiteActor(t, fixtures.MeridianOwnerMonaEmail), fixtures.MeridianPublicID)

	failed, ok := itemAbout(items, siteapi.SiteAttentionKindEnrollmentsFailed, fixtures.AutomationMeridianWelcomeID)
	require.True(t, ok)
	assert.EqualValues(t, 2, failed.Count, "the run of eight days ago is no longer an item")
	assert.Equal(t, siteapi.SiteAttentionSeverityNotice, failed.Severity)
}

// What asks for attention leaves archived rows out (ADR 0099), as it does an archived Broadcast.
// Fixtures: an Enrollment of Meridian's archived Automation failed within the last hour.
func TestTheFailedRunsOfAnArchivedAutomationAreNoItem(t *testing.T) {
	env := testhelper.Setup(t)

	items := attentionOf(t, env.SiteActor(t, fixtures.MeridianOwnerMonaEmail), fixtures.MeridianPublicID)

	_, failed := itemAbout(items, siteapi.SiteAttentionKindEnrollmentsFailed, fixtures.AutomationMeridianArchivedID)
	assert.False(t, failed)
}

func TestABroadcastThatFailedThisWeekIsAnItemSinceItFailed(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	member := env.SiteActor(t, fixtures.OwnerJohnEmail)
	failedAt := time.Now().AddDate(0, 0, -2).Truncate(time.Second)
	require.NoError(t, acme.Broadcast().UpdateOneID(fixtures.BroadcastFailedID).SetFailedAt(failedAt).Exec(t.Context()))

	items := attentionOf(t, member, fixtures.AcmePublicID)

	failed, ok := itemAbout(items, siteapi.SiteAttentionKindBroadcastFailed, fixtures.BroadcastFailedID)
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteAttentionSeverityNotice, failed.Severity)
	require.True(t, failed.Since.Set)
	assert.True(t, failedAt.Equal(time.Time(failed.Since.Value)), "since is when it failed, got %v", failed.Since.Value)
}

func TestABroadcastThatFailedLongAgoIsNoItemAfterAnEdit(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	member := env.SiteActor(t, fixtures.OwnerJohnEmail)
	require.NoError(t, acme.Broadcast().UpdateOneID(fixtures.BroadcastFailedID).
		SetFailedAt(time.Now().AddDate(0, 0, -8)).Exec(t.Context()))
	require.NoError(t, acme.Broadcast().UpdateOneID(fixtures.BroadcastFailedID).SetName("Renamed today").Exec(t.Context()))

	items := attentionOf(t, member, fixtures.AcmePublicID)

	_, failed := itemAbout(items, siteapi.SiteAttentionKindBroadcastFailed, fixtures.BroadcastFailedID)
	assert.False(t, failed, "a rename does not bring back a failure of eight days ago")
}

// A verified Sending domain whose last check found no DMARC record is not bulk-ready (ADR 0012):
// a notice that names it, until a check finds the record. The check is the "re-check now" of the
// Sending domain, which answers with when the record was found.
func TestAVerifiedDomainWithoutADMARCRecordIsANoticeUntilOneIsPublished(t *testing.T) {
	zone := dnstest.NewZone(t)
	env := testhelper.Setup(t, testhelper.WithTXTLookup(zone.LookupTXT))
	ctx := t.Context()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	dom, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	zone.PublishTXT(t, "sphericon._domainkey.mail.acme.com.", dom.DkimPublicKey)
	recheck := func() *siteapi.SiteSendingDomainResource {
		t.Helper()
		res, err := c.SiteSendingDomainsVerify(ctx, siteapi.SiteSendingDomainsVerifyParams{
			WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.SendingDomainVerifiedID),
		})
		require.NoError(t, err)
		d := res
		return d
	}

	missing := recheck()

	assert.True(t, missing.Verified, "DMARC never gates sending")
	_, looked := missing.DmarcCheckedAt.Get()
	_, found := missing.DmarcFoundAt.Get()
	assert.True(t, looked, "the DMARC lookup answered")
	assert.False(t, found, "the check found no DMARC record")
	noticed := attentionOf(t, c, fixtures.AcmePublicID)
	notice, ok := itemAbout(noticed, siteapi.SiteAttentionKindSendingDomainNotBulkReady, fixtures.SendingDomainVerifiedID)
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteAttentionSeverityNotice, notice.Severity)
	assert.Equal(t, fixtures.SendingDomainVerifiedDomain, notice.Subject.Value.Name)

	zone.PublishTXT(t, "_dmarc.acme.com.", "v=DMARC1; p=none;")
	published := recheck()

	_, found = published.DmarcFoundAt.Get()
	assert.True(t, found, "the organizational domain's record covers mail.acme.com")
	read, err := c.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{
		WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.SendingDomainVerifiedID),
	})
	require.NoError(t, err)
	readRes := read
	assert.Equal(t, published.DmarcFoundAt, readRes.DmarcFoundAt, "a read of the domain shows the same moment")
	cleared := attentionOf(t, c, fixtures.AcmePublicID)
	_, still := itemAbout(cleared, siteapi.SiteAttentionKindSendingDomainNotBulkReady, fixtures.SendingDomainVerifiedID)
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

	items := attentionOf(t, env.SiteActor(t, fixtures.MeridianOwnerMonaEmail), fixtures.MeridianPublicID)

	got := kinds(items)
	assert.NotContains(t, got, siteapi.SiteAttentionKindSendingDomainNotBulkReady)
}

// withSendCap is the test env with Capton's Send cap set to perDay messages per 24 hours, its bucket
// full, and its two Broadcasts, each to the same three Contacts, placed to start in 2 and 3 hours
// from now (the fixtures' own times are as of when the template database was loaded).
func withSendCap(t *testing.T, perDay int) *testhelper.TestEnv {
	t.Helper()
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.CaptonID).SetSendCap(perDay).ExecX(t.Context())
	capton := env.DB.Scoped(fixtures.CaptonID)
	now := time.Now().Truncate(time.Second) // a Timestamp of the API is whole seconds
	require.NoError(t, capton.Broadcast().UpdateOneID(fixtures.BroadcastCaptonSoonerID).SetScheduledAt(now.Add(2*time.Hour)).Exec(t.Context()))
	require.NoError(t, capton.Broadcast().UpdateOneID(fixtures.BroadcastCaptonLaterID).SetScheduledAt(now.Add(3*time.Hour)).Exec(t.Context()))
	return env
}

// startOf is when the Capton Broadcast id is scheduled to start.
func startOf(t *testing.T, env *testhelper.TestEnv, id int64) time.Time {
	t.Helper()
	b, err := env.DB.Scoped(fixtures.CaptonID).Broadcast().Get(t.Context(), id)
	require.NoError(t, err)
	return *b.ScheduledAt
}

// An audience of 3 under a Send cap of 2: the full bucket lets 2 out at the start, and the third
// waits for half of the bucket to refill, 12 hours at 2 per 24 hours.
func TestAScheduledBroadcastLargerThanTheSendCapIsAnItemWithItsFinish(t *testing.T) {
	env := withSendCap(t, 2)
	finish := startOf(t, env, fixtures.BroadcastCaptonSoonerID).Add(12 * time.Hour)

	items := attentionOf(t, env.SiteActor(t, fixtures.CaptonOwnerCleoEmail), fixtures.CaptonPublicID)

	item, ok := itemAbout(items, siteapi.SiteAttentionKindScheduledBroadcastOverSendCap, fixtures.BroadcastCaptonSoonerID)
	require.True(t, ok)
	got := time.Time(item.EstimatedCompletionAt.Value)
	assert.Equal(t, siteapi.SiteAttentionSeverityDegraded, item.Severity)
	assert.Equal(t, fixtures.BroadcastCaptonSoonerName, item.Subject.Value.Name)
	assert.Equal(t, int32(3), item.Count, "the estimated audience")
	assert.True(t, finish.Equal(got), "expected to finish at %v, got %v", finish, got)
}

// Under a Send cap of 6 both Broadcasts, 6 Contacts in all, go out from the full bucket.
func TestScheduledBroadcastsTheSendCapLetsOutAreNoItem(t *testing.T) {
	env := withSendCap(t, 6)

	items := attentionOf(t, env.SiteActor(t, fixtures.CaptonOwnerCleoEmail), fixtures.CaptonPublicID)

	assert.Empty(t, items)
}

// Under a Send cap of 4 each Broadcast fits alone, but they draw from the same bucket: the later one
// waits for the earlier one's audience too, 2 of 6 beyond the bucket, 12 hours at 4 per 24 hours.
func TestScheduledBroadcastsAreJudgedTogetherInTheOrderTheyStart(t *testing.T) {
	env := withSendCap(t, 4)
	finish := startOf(t, env, fixtures.BroadcastCaptonLaterID).Add(12 * time.Hour)

	items := attentionOf(t, env.SiteActor(t, fixtures.CaptonOwnerCleoEmail), fixtures.CaptonPublicID)

	require.Len(t, items, 1)
	later := items[0]
	got := time.Time(later.EstimatedCompletionAt.Value)
	assert.Equal(t, siteapi.SiteAttentionKindScheduledBroadcastOverSendCap, later.Kind)
	assert.Equal(t, siteapi.EntityId(fixtures.BroadcastCaptonLaterID), later.Subject.Value.ID)
	assert.Equal(t, int32(3), later.Count, "its own audience, not the sum")
	assert.True(t, finish.Equal(got), "expected to finish at %v, got %v", finish, got)
}

// A Broadcast already sending draws its remaining recipients from the bucket before any that has yet
// to start: under a Send cap of 6, its 3 and the sooner one's 3 fill the bucket, and the later one's 3
// wait for half of it to refill, 12 hours at 6 per 24 hours.
func TestTheRecipientsLeftToASendingBroadcastComeBeforeTheScheduledOnes(t *testing.T) {
	env := withSendCap(t, 6)
	lift := env.DB.Scoped(fixtures.CaptonID).Broadcast().UpdateOneID(fixtures.BroadcastCaptonSendingID).ClearHoldReason()
	require.NoError(t, lift.Exec(t.Context()))
	finish := startOf(t, env, fixtures.BroadcastCaptonLaterID).Add(12 * time.Hour)

	items := attentionOf(t, env.SiteActor(t, fixtures.CaptonOwnerCleoEmail), fixtures.CaptonPublicID)

	require.Len(t, items, 1)
	later := items[0]
	got := time.Time(later.EstimatedCompletionAt.Value)
	assert.Equal(t, siteapi.EntityId(fixtures.BroadcastCaptonLaterID), later.Subject.Value.ID)
	assert.True(t, finish.Equal(got), "expected to finish at %v, got %v", finish, got)
}

func TestABroadcastScheduledAfterTheNext24HoursIsNoItem(t *testing.T) {
	env := withSendCap(t, 2)
	capton := env.DB.Scoped(fixtures.CaptonID)
	later := time.Now().Add(30 * time.Hour)
	require.NoError(t, capton.Broadcast().UpdateOneID(fixtures.BroadcastCaptonSoonerID).SetScheduledAt(later).Exec(t.Context()))
	require.NoError(t, capton.Broadcast().UpdateOneID(fixtures.BroadcastCaptonLaterID).SetScheduledAt(later).Exec(t.Context()))

	items := attentionOf(t, env.SiteActor(t, fixtures.CaptonOwnerCleoEmail), fixtures.CaptonPublicID)

	assert.Empty(t, items)
}

// A held Broadcast is a blocked item of its own and takes nothing from the bucket while it is held:
// the later one, alone, fits a Send cap of 4.
func TestAHeldScheduledBroadcastIsNoSendCapItemAndTakesNothingFromTheOthers(t *testing.T) {
	env := withSendCap(t, 4)
	hold := env.DB.Scoped(fixtures.CaptonID).Broadcast().UpdateOneID(fixtures.BroadcastCaptonSoonerID).SetHoldReason(holdreason.NoIntegration)
	require.NoError(t, hold.Exec(t.Context()))

	items := attentionOf(t, env.SiteActor(t, fixtures.CaptonOwnerCleoEmail), fixtures.CaptonPublicID)

	assert.Equal(t, []siteapi.SiteAttentionKind{siteapi.SiteAttentionKindBroadcastHeld}, kinds(items))
}

func TestWithoutASendCapNoScheduledBroadcastIsAnItem(t *testing.T) {
	env := testhelper.Setup(t)

	items := attentionOf(t, env.SiteActor(t, fixtures.CaptonOwnerCleoEmail), fixtures.CaptonPublicID)

	assert.Empty(t, items)
}

// cannotSendOf is the automationCannotSend item about the Automation id, and whether there is one.
func cannotSendOf(items []siteapi.SiteAttentionItem, id int64) (siteapi.SiteAttentionItem, bool) {
	return itemAbout(items, siteapi.SiteAttentionKindAutomationCannotSend, id)
}

// Fixtures: the Jobs emails-then-tag Automation is active; its first email step goes through the
// Workspace default, its second through the Mailgun relay. This test archives the relay: one held
// step is enough.
func TestAnActiveAutomationWithAStepWhoseIntegrationIsArchivedCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	member := env.SiteActor(t, fixtures.OwnerJohnEmail)
	_, before := cannotSendOf(attentionOf(t, member, fixtures.AcmePublicID), fixtures.AutomationJobsEmailsThenTagID)
	require.NoError(t, acme.Integration().UpdateOneID(fixtures.IntegrationAcmeMailgunID).
		SetDiscardedAt(time.Now()).Exec(t.Context()))

	items := attentionOf(t, member, fixtures.AcmePublicID)

	assert.False(t, before, "it sends while its Integration is kept")
	held, ok := cannotSendOf(items, fixtures.AutomationJobsEmailsThenTagID)
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteAttentionSeverityBlocked, held.Severity)
	assert.Equal(t, siteapi.SiteAttentionSubject{ID: fixtures.AutomationJobsEmailsThenTagID, Name: fixtures.AutomationJobsEmailsThenTagName}, held.Subject.Value)
	assert.Equal(t, siteapi.HoldReasonNoIntegration, held.HoldReason.Value)
	assert.False(t, held.Since.Set, "no moment is stored for when a route started holding")
	_, welcome := cannotSendOf(items, fixtures.AutomationWelcomeSeriesID)
	assert.False(t, welcome, "the Automations on the default still send")
}

// Fixtures: the Workspace default's sender sends from hello@codebasics.dev.
func TestAnActiveAutomationWhoseFromDomainIsNotVerifiedCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, acme.SendingDomain().UpdateOneID(fixtures.SendingDomainCodebasicsID).
		SetVerificationLostAt(time.Now()).Exec(t.Context()))

	items := attentionOf(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID)

	held, ok := cannotSendOf(items, fixtures.AutomationWelcomeSeriesID)
	require.True(t, ok)
	assert.Equal(t, siteapi.HoldReasonUnverifiedDomain, held.HoldReason.Value)
	_, holdDemo := cannotSendOf(items, fixtures.AutomationHoldDemoID)
	assert.True(t, holdDemo, "every Automation that cannot send is an item of its own")
}

func TestAnActiveAutomationWhoseProviderStoppedTheAccountCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, acme.Integration().UpdateOneID(fixtures.IntegrationAcmeDefaultID).
		SetProviderEnforcement(providers.EnforcementShutdown).Exec(t.Context()))

	items := attentionOf(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID)

	held, ok := cannotSendOf(items, fixtures.AutomationWelcomeSeriesID)
	require.True(t, ok)
	assert.Equal(t, siteapi.HoldReasonProviderStopped, held.HoldReason.Value)
}

// Fixtures: the provider stopped the Integration each of Meridian's Automations below sends through,
// so each Automation left out is left out for what it is.
func TestOnlyAnActiveAutomationThatSendsEmailInAWorkspaceThatSendsIsAnItem(t *testing.T) {
	env := testhelper.Setup(t)

	items := attentionOf(t, env.SiteActor(t, fixtures.MeridianOwnerMonaEmail), fixtures.MeridianPublicID)

	_, paused := cannotSendOf(items, fixtures.AutomationMeridianInactiveID)
	assert.False(t, paused, "a paused Automation sends nothing")
	_, archived := cannotSendOf(items, fixtures.AutomationMeridianArchivedID)
	assert.False(t, archived, "an archived Automation sends nothing")
	_, tags := cannotSendOf(items, fixtures.AutomationMeridianNoEmailID)
	assert.False(t, tags, "an Automation with no email step sends through no Integration")
	_, stopped := cannotSendOf(items, fixtures.AutomationMeridianStoppedID)
	assert.True(t, stopped, "an active one with an email step is still an item")
}

// Fixtures: Meridian's draft-only Automation has a live version with a wait step and a draft with an
// email step through an Integration its provider stopped.
func TestAnEmailStepOnlyInTheDraftMakesNoItem(t *testing.T) {
	env := testhelper.Setup(t)

	items := attentionOf(t, env.SiteActor(t, fixtures.MeridianOwnerMonaEmail), fixtures.MeridianPublicID)

	_, draft := cannotSendOf(items, fixtures.AutomationMeridianDraftOnlyID)
	assert.False(t, draft, "a draft sends nothing until it is published")
}

// The default is stopped too, so the suspension is what leaves the item out.
func TestASuspendedWorkspaceShowsNoAutomationThatCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, acme.Integration().UpdateOneID(fixtures.IntegrationAcmeDefaultID).
		SetProviderEnforcement(providers.EnforcementShutdown).Exec(t.Context()))
	suspended, err := suspension.SuspendWorkspace(t.Context(), env.Bus, fixtures.AcmeID, suspension.System, "abuse")
	require.NoError(t, err)
	require.True(t, suspended)

	items := attentionOf(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID)

	assert.NotContains(t, kinds(items), siteapi.SiteAttentionKindAutomationCannotSend, "the suspension is the layout's banner")
}
