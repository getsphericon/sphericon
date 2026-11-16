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

// Marketing email is armed only with a postal address for its footer (ADR 0156).

// setPostalAddress saves the Workspace's postal address as its member c does in the settings.
func setPostalAddress(t *testing.T, c *siteapi.Client, publicID siteapi.WorkspaceId, address string) (*siteapi.SiteWorkspaceResource, error) {
	t.Helper()
	ws, err := c.SiteWorkspacesGet(t.Context(), siteapi.SiteWorkspacesGetParams{WorkspaceId: publicID})
	require.NoError(t, err)
	return c.SiteWorkspacesUpdate(t.Context(), &siteapi.SiteUpdateWorkspaceInput{
		Revision: ws.Revision, Name: siteapi.Name(ws.Name), PostalAddress: siteapi.NewOptString(address),
	}, siteapi.SiteWorkspacesUpdateParams{WorkspaceId: publicID})
}

// usedByIDs is the id of every row a 409 names as what holds the subject.
func usedByIDs(p siteapi.ProblemDetails) []siteapi.EntityId {
	return lo.Map(p.UsedBy, func(u siteapi.UsedBy, _ int) siteapi.EntityId { return u.ID })
}

func TestSiteSchedulingAnEmailBroadcastNeedsAPostalAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	env.ClearPostalAddress(t, fixtures.AcmeID)
	params := siteapi.SiteBroadcastsScheduleParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastDraftID)}
	at := siteapi.Timestamp(time.Now().Add(time.Hour))

	_, refusedErr := c.SiteBroadcastsSchedule(t.Context(), &siteapi.ScheduleBroadcastInput{Revision: siteapi.Revision(env.BroadcastRevision(t.Context(), t, int64(fixtures.BroadcastDraftID))), ScheduledAt: at}, params)
	_, err := setPostalAddress(t, c, fixtures.AcmePublicID, "  1 Infinite Loop\nCupertino  ")
	require.NoError(t, err)
	scheduled, err := c.SiteBroadcastsSchedule(t.Context(), &siteapi.ScheduleBroadcastInput{Revision: siteapi.Revision(env.BroadcastRevision(t.Context(), t, int64(fixtures.BroadcastDraftID))), ScheduledAt: at}, params)

	refused := testhelper.SiteRefusal(t, refusedErr, http.StatusConflict)
	assert.Equal(t, siteapi.ProblemCodePostalAddressRequired, refused.Code.Value)
	require.NoError(t, err)
	assert.Equal(t, siteapi.SiteBroadcastStatusScheduled, scheduled.Status)
	stored := env.DB.Workspace.GetX(t.Context(), fixtures.AcmeID).PostalAddress
	assert.Equal(t, "1 Infinite Loop\nCupertino", stored, "the address is kept trimmed, its lines as written")
}

func TestSitePublishingAnEmailAutomationNeedsAPostalAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	env.ClearPostalAddress(t, fixtures.AcmeID)

	_, err := c.SiteAutomationsPublish(t.Context(), &siteapi.RevisionInput{Revision: siteapi.Revision(env.AutomationRevision(t.Context(), t, int64(fixtures.AutomationWinBackID)))},
		siteapi.SiteAutomationsPublishParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AutomationWinBackID)})

	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Equal(t, siteapi.ProblemCodePostalAddressRequired, refused.Code.Value)
}

// Acme's BroadcastScheduled is an email Broadcast still to send: clearing the address would send it
// without one, so the save is refused, names it, and the address stays.
func TestSiteClearingThePostalAddressIsRefusedWhileMarketingEmailIsArmed(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	before := env.DB.Workspace.GetX(t.Context(), fixtures.AcmeID).PostalAddress

	_, err := setPostalAddress(t, c, fixtures.AcmePublicID, " \n ")

	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	named := usedByIDs(refused)
	assert.Contains(t, named, idStr(fixtures.BroadcastScheduledID))
	after := env.DB.Workspace.GetX(t.Context(), fixtures.AcmeID).PostalAddress
	assert.Equal(t, before, after)
}

// Globex has no email Broadcast to send and no active Automation: its address may be cleared.
func TestSiteThePostalAddressIsClearedWithNothingArmed(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)

	saved, err := setPostalAddress(t, c, fixtures.GlobexPublicID, "")

	require.NoError(t, err)
	assert.Empty(t, saved.PostalAddress)
}

// A Test send goes to the sender, not to recipients (ADR 0132): it needs no address.
func TestSiteTestSendGoesOutWithoutAPostalAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)
	env.ClearPostalAddress(t, fixtures.GlobexID)

	_, err := testSendAsJane(env, c, fixtures.OwnerJaneEmail)

	require.NoError(t, err)
	sent := env.SystemMail.Messages()
	require.Len(t, sent, 1)
	assert.Equal(t, fixtures.OwnerJaneEmail, sent[0].To)
}

// Acme has email Integrations: while its address is empty the Overview says so.
func TestSiteAWorkspaceWithAnEmailIntegrationAndNoPostalAddressNeedsAttention(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)
	withAddress := kinds(attentionOf(t, c, fixtures.AcmePublicID))
	env.ClearPostalAddress(t, fixtures.AcmeID)

	without := kinds(attentionOf(t, c, fixtures.AcmePublicID))

	assert.NotContains(t, withAddress, siteapi.SiteAttentionKindPostalAddressMissing)
	assert.Contains(t, without, siteapi.SiteAttentionKindPostalAddressMissing)
}

// A Test send through the Workspace's own Integration builds the marketing footer, and goes out
// without the address all the same.
func TestSiteTestSendThroughTheWorkspacesIntegrationGoesOutWithoutAPostalAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	env.ClearPostalAddress(t, fixtures.AcmeID)

	_, err := c.SiteBroadcastsTestSend(t.Context(), &siteapi.TestSendBroadcastInput{Email: siteapi.NewOptEmailAddress("anyone@example.com")},
		siteapi.SiteBroadcastsTestSendParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastDraftID)})

	require.NoError(t, err)
	sent := env.CustomerMail.Messages()
	require.Len(t, sent, 1, "through the Workspace's Integration")
	assert.Equal(t, "anyone@example.com", sent[0].To)
}
