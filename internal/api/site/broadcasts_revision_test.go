package site_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The editor loaded the draft; then its segment was changed through PUT …/segment. Send now from
// that editor names the revision it showed: a 409 revision_changed, and nothing is sent (ADR 0119).
func TestSiteSendNowFromAnEditorLoadedBeforeASegmentChangeIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastDraftID)}
	shown, err := c.SiteBroadcastsGet(ctx, params)
	require.NoError(t, err)

	api := env.ExternalScoped(t, apitokens.ScopeBroadcastsWrite)
	_, err = api.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{
		Revision: externalapi.Revision(shown.Revision), SegmentId: externalapi.NewNilEntityId(externalapi.EntityId(fixtures.SegmentProPlanID)),
	}, externalapi.BroadcastsSetSegmentParams{ID: externalapi.EntityId(fixtures.BroadcastDraftID)})
	require.NoError(t, err)

	_, err = c.SiteBroadcastsSend(ctx, &siteapi.RevisionInput{Revision: shown.Revision},
		siteapi.SiteBroadcastsSendParams(params))

	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Equal(t, siteapi.ProblemCodeRevisionChanged, refused.Code.Value)
	stored := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)
	assert.Equal(t, broadcast.StatusDraft, stored.Status, "nothing is sent")
	assert.Equal(t, int64(fixtures.SegmentProPlanID), *stored.SegmentID)
}

// Two Saves of a draft made on one revision: the second is a 409 revision_changed and the draft is
// the first Save's. A Schedule on the revision before the first Save is refused the same way.
func TestSiteBroadcastWritesOnAStaleRevisionAreRefused(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	update := siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastDraftID)}
	read := siteapi.Revision(env.BroadcastRevision(ctx, t, fixtures.BroadcastDraftID))

	first, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Revision: read, Name: siteapi.NewOptName("First")}, update)
	require.NoError(t, err)
	_, err = c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Revision: read, Name: siteapi.NewOptName("Second")}, update)
	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Equal(t, siteapi.ProblemCodeRevisionChanged, refused.Code.Value)

	_, err = c.SiteBroadcastsSchedule(ctx, &siteapi.ScheduleBroadcastInput{Revision: read, ScheduledAt: siteapi.Timestamp(time.Now().Add(time.Hour))},
		siteapi.SiteBroadcastsScheduleParams(update))
	refused = testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Equal(t, siteapi.ProblemCodeRevisionChanged, refused.Code.Value)

	stored := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)
	assert.Equal(t, "First", stored.Name)
	assert.Equal(t, broadcast.StatusDraft, stored.Status)
	assert.EqualValues(t, first.Revision, stored.Revision)
}

// A Broadcast past draft is refused for what it is, whatever revision the request names: reading it
// again would change nothing, so it is not a revision_changed.
func TestSiteBroadcastPastDraftIsRefusedForItsStatusBeforeItsRevision(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	stale := siteapi.Revision(env.BroadcastRevision(ctx, t, fixtures.BroadcastSentID) + 5)

	_, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Revision: stale, Name: siteapi.NewOptName("x")},
		siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastSentID)})
	notDraft := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.NotEqual(t, siteapi.ProblemCodeRevisionChanged, notDraft.Code.Value)

	_, err = c.SiteBroadcastsSend(ctx, &siteapi.RevisionInput{Revision: stale},
		siteapi.SiteBroadcastsSendParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastSentID)})
	notSendable := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.NotEqual(t, siteapi.ProblemCodeRevisionChanged, notSendable.Code.Value)
}
