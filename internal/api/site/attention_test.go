package site_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/needsattentionreport"
)

// What each item of the report is and when it is one is the use case's
// (internal/usecase/needsattentionreport); these tests hold who reads it and how an item reads.

// attentionOf is what needs attention in the Workspace, as the member c reads it.
func attentionOf(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId) []siteapi.SiteAttentionItem {
	t.Helper()
	res, err := c.SiteAttentionGet(t.Context(), siteapi.SiteAttentionGetParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	return res.Items
}

// reportOf is what needs attention in the Workspace as the use case reads it, for a test of how the
// response reads its items.
func reportOf(t *testing.T, env *testhelper.TestEnv, workspaceID int64) []needsattentionreport.Item {
	t.Helper()
	items, err := testhelper.Module[*needsattentionreport.UseCase](env).Items(t.Context(), env.DB.Scoped(workspaceID))
	require.NoError(t, err)
	return items
}

// kinds is the kind of every item, in the order the list gives them.
func kinds(items []siteapi.SiteAttentionItem) []siteapi.SiteAttentionKind {
	return lo.Map(items, func(i siteapi.SiteAttentionItem, _ int) siteapi.SiteAttentionKind { return i.Kind })
}

// Fixtures: Acme's Sending domain lost.acme.com lost its DKIM record. A plain member reads the
// report as it is, in its order, as an Owner does.
func TestEveryMemberReadsWhatNeedsAttention(t *testing.T) {
	env := testhelper.Setup(t)
	report := lo.Map(reportOf(t, env, fixtures.AcmeID), func(i needsattentionreport.Item, _ int) siteapi.SiteAttentionKind {
		return siteapi.SiteAttentionKind(i.Kind)
	})

	items := attentionOf(t, env.SiteActor(t, fixtures.MemberMaryEmail), fixtures.AcmePublicID)

	assert.Equal(t, report, kinds(items))
	lost, ok := testhelper.AttentionItemAbout(items, siteapi.SiteAttentionKindSendingDomainVerificationLost, fixtures.SendingDomainLostID)
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteAttentionSeverityBlocked, lost.Severity)
	assert.Equal(t, siteapi.SiteAttentionSubject{ID: fixtures.SendingDomainLostID, Name: fixtures.SendingDomainLostDomain}, lost.Subject.Value)
	assert.EqualValues(t, 1, lost.Count)
	assert.True(t, lost.Since.Set, "the moment the record was found gone")
	assert.False(t, lost.HoldReason.Set)
	assert.False(t, lost.EstimatedCompletionAt.Set)
}

// A User who is no member of the Workspace finds no report there.
func TestAnOutsiderReadsNoReport(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.SiteActor(t, fixtures.OutsiderOscarEmail).SiteAttentionGet(t.Context(),
		siteapi.SiteAttentionGetParams{WorkspaceId: fixtures.AcmePublicID})

	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}

// The Send cap is the Workspace's own and has no start: its item reads with neither.
func TestAnItemOfTheWorkspaceItselfReadsWithoutASubjectOrAStart(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(1).ExecX(t.Context())

	items := attentionOf(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID)

	capped, ok := lo.Find(items, func(i siteapi.SiteAttentionItem) bool { return i.Kind == siteapi.SiteAttentionKindSendCapSpent })
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteAttentionSeverityDegraded, capped.Severity)
	assert.False(t, capped.Subject.Set)
	assert.False(t, capped.Since.Set)
}

// Fixtures: Meridian's scheduled Broadcast from news.meridian.test is held for that unverified domain.
func TestAHeldBroadcastReadsWithItsHoldReason(t *testing.T) {
	env := testhelper.Setup(t)

	items := attentionOf(t, env.SiteActor(t, fixtures.MeridianOwnerMonaEmail), fixtures.MeridianPublicID)

	held, ok := testhelper.AttentionItemAbout(items, siteapi.SiteAttentionKindBroadcastHeld, fixtures.BroadcastMeridianFromNewsID)
	require.True(t, ok)
	assert.Equal(t, siteapi.HoldReasonUnverifiedDomain, held.HoldReason.Value)
}

// Fixtures: Capton's sooner Broadcast goes to three Contacts, more than a Send cap of 2 lets out at
// its start. The item reads with the report's audience and finish, the finish in whole seconds.
func TestABroadcastOverTheSendCapReadsWithItsAudienceAndItsFinish(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.CaptonID).SetSendCap(2).ExecX(t.Context())
	start := time.Now().Add(2 * time.Hour)
	capton := env.DB.Scoped(fixtures.CaptonID)
	require.NoError(t, capton.Broadcast().UpdateOneID(fixtures.BroadcastCaptonSoonerID).SetScheduledAt(start).Exec(t.Context()))
	require.NoError(t, capton.Broadcast().UpdateOneID(fixtures.BroadcastCaptonLaterID).SetScheduledAt(start.Add(30*time.Hour)).Exec(t.Context()))
	held, ok := lo.Find(reportOf(t, env, fixtures.CaptonID), func(i needsattentionreport.Item) bool {
		return i.Kind == needsattentionreport.KindScheduledBroadcastOverSendCap
	})
	require.True(t, ok)

	items := attentionOf(t, env.SiteActor(t, fixtures.CaptonOwnerCleoEmail), fixtures.CaptonPublicID)

	item, ok := testhelper.AttentionItemAbout(items, siteapi.SiteAttentionKindScheduledBroadcastOverSendCap, fixtures.BroadcastCaptonSoonerID)
	require.True(t, ok)
	assert.EqualValues(t, held.Count, item.Count)
	assert.Equal(t, fixtures.BroadcastCaptonSoonerName, item.Subject.Value.Name)
	got := time.Time(item.EstimatedCompletionAt.Value)
	assert.True(t, held.EstimatedCompletionAt.Truncate(time.Second).Equal(got), "expected %v in whole seconds, got %v", *held.EstimatedCompletionAt, got)
}
