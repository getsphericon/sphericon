package site_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The console names a Broadcast's audience too (ADR 0149): a create without segmentId is refused
// and stores nothing.
func TestSiteBroadcastCreateWithoutAnAudienceIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	unnamed := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.EditBody(func(body map[string]any) {
		delete(body, "segmentId")
	}))
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	list := siteapi.SiteBroadcastsListParams{WorkspaceId: fixtures.AcmePublicID}
	before, err := c.SiteBroadcastsList(ctx, list)
	require.NoError(t, err)

	_, err = unnamed.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name:      "Spring news",
		Content:   testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{}),
		SegmentId: siteapi.NilEntityId{Null: true},
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})

	after, listErr := c.SiteBroadcastsList(ctx, list)

	require.NoError(t, listErr)
	refused := testhelper.SiteRefusal(t, err, http.StatusBadRequest)
	detail := refused.Detail.Or("")
	assert.Contains(t, detail, "segmentId (field required)")
	assert.Equal(t, before.TotalItems, after.TotalItems, "a refused create stores nothing")
}

// Schedule answers the Recipients the Broadcast would take at that moment (ADR 0149): the figure
// the preview gives for its audience on its Channel, which leaves a suppressed Contact out.
func TestSiteScheduleAnswersTheRecipientsAtThatMoment(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	env.SuppressDestination(ctx, fixtures.ContactLiamEmail)
	preview, err := c.SiteSegmentsPreview(ctx, &siteapi.SitePreviewSegmentInput{
		Condition: siteapi.NewOptConditionGroupInput(testhelper.SiteRuleGroup(t,
			`{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:plan","operator":"eq","value":"pro"}]}`)),
		Channel: siteapi.NewOptBroadcastChannel(siteapi.BroadcastChannelEmail),
	}, siteapi.SiteSegmentsPreviewParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	require.Less(t, preview.Recipients.Value, preview.Count, "the suppressed pro Contact is matched but not reached")
	id := idStr(fixtures.BroadcastProSegmentID)

	scheduled, err := c.SiteBroadcastsSchedule(ctx, &siteapi.ScheduleBroadcastInput{
		Revision:    siteapi.Revision(env.BroadcastRevision(ctx, t, fixtures.BroadcastProSegmentID)),
		ScheduledAt: siteapi.Timestamp(time.Now().Add(time.Hour)),
	}, siteapi.SiteBroadcastsScheduleParams{WorkspaceId: fixtures.AcmePublicID, ID: id})

	require.NoError(t, err)
	assert.Equal(t, preview.Recipients, scheduled.Recipients)
}
