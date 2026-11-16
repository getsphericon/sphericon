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

// Fixtures: Acme's Sending domain lost.acme.com lost its DKIM record, and two of its Webhook
// endpoints disabled themselves. A plain member reads them as an Owner does.
func TestEveryMemberReadsWhatNeedsAttention(t *testing.T) {
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

	held, ok := itemAbout(items, siteapi.SiteAttentionKindBroadcastHeld, fixtures.BroadcastMeridianFromNewsID)
	require.True(t, ok)
	assert.Equal(t, siteapi.HoldReasonUnverifiedDomain, held.HoldReason.Value)
}

// Fixtures: Capton's sooner Broadcast goes to three Contacts. Under a Send cap of 2 it finishes 12
// hours after its start, which reads in whole seconds.
func TestABroadcastOverTheSendCapReadsWithItsAudienceAndItsFinish(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.CaptonID).SetSendCap(2).ExecX(t.Context())
	start := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	capton := env.DB.Scoped(fixtures.CaptonID)
	require.NoError(t, capton.Broadcast().UpdateOneID(fixtures.BroadcastCaptonSoonerID).SetScheduledAt(start).Exec(t.Context()))
	require.NoError(t, capton.Broadcast().UpdateOneID(fixtures.BroadcastCaptonLaterID).SetScheduledAt(start.Add(30*time.Hour)).Exec(t.Context()))

	items := attentionOf(t, env.SiteActor(t, fixtures.CaptonOwnerCleoEmail), fixtures.CaptonPublicID)

	item, ok := itemAbout(items, siteapi.SiteAttentionKindScheduledBroadcastOverSendCap, fixtures.BroadcastCaptonSoonerID)
	require.True(t, ok)
	assert.Equal(t, int32(3), item.Count)
	assert.Equal(t, fixtures.BroadcastCaptonSoonerName, item.Subject.Value.Name)
	got := time.Time(item.EstimatedCompletionAt.Value)
	assert.True(t, start.Add(12*time.Hour).Equal(got), "expected to finish at %v, got %v", start.Add(12*time.Hour), got)
}
