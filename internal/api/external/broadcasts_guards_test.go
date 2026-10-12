package external_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestExternalBroadcastsCreateRefusesABlankName(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, authorScopes...)
	before, err := env.DB.Broadcast.Query().Where(broadcast.WorkspaceID(fixtures.AcmeID)).Count(ctx)
	require.NoError(t, err)

	res, err := c.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{Name: ""})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsCreateUnprocessableEntity{}, res)

	after, err := env.DB.Broadcast.Query().Where(broadcast.WorkspaceID(fixtures.AcmeID)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, before, after, "nothing is stored")
}

// A segment id that is not an int64 is the same 422 as an unknown segment, and
// the audience is left as it was.
func TestExternalBroadcastsSetAudienceRefusesAnUnparsableSegment(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, authorScopes...)

	res, err := c.BroadcastsSetAudience(ctx, &externalapi.SetBroadcastAudienceInput{SegmentId: externalapi.NewNilEntityId(overflowID)},
		externalapi.BroadcastsSetAudienceParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsSetAudienceUnprocessableEntity{}, res)

	stored, err := env.DB.Broadcast.Get(ctx, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	assert.Nil(t, stored.SegmentID)
}

// A provider failure on a test send is a 422 with the cause, never a 500, and
// the broadcast stays a draft.
func TestExternalBroadcastTestSendReportsAFailedSend(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, authorScopes...)
	env.CustomerMail.SetErr(errors.New("smtp unavailable"))

	res, err := c.BroadcastsTestSend(ctx, &externalapi.TestSendBroadcastInput{Email: "qa@test.dev"},
		externalapi.BroadcastsTestSendParams{ID: draftBroadcast})
	require.NoError(t, err)
	unprocessable, ok := res.(*externalapi.BroadcastsTestSendUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, unprocessable.Detail.Value, "smtp unavailable")
	assert.Empty(t, env.CustomerMail.Messages())
	assert.Equal(t, broadcast.StatusDraft, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Status)
}

// The report carries the reason a broadcast is held.
func TestExternalBroadcastReportCarriesTheHoldReason(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, authorScopes...)
	env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetHoldReason("workspace_suspended").ExecX(ctx)

	res, err := c.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: draftBroadcast})
	require.NoError(t, err)
	report, ok := res.(*externalapi.BroadcastReport)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, "workspace_suspended", report.HoldReason.Value)
}
