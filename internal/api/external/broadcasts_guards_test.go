package external_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/holdreason"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A blank name, empty or whitespace only, is refused with the same 422 on create and update, and
// nothing is stored or changed.
func TestExternalBroadcastsRefuseABlankName(t *testing.T) {
	for _, name := range []string{"", "   "} {
		t.Run("create "+name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := context.Background()
			c := env.ExternalScoped(t, authorScopes...)
			before, err := env.DB.Broadcast.Query().Where(broadcast.WorkspaceID(fixtures.AcmeID)).Count(ctx)
			require.NoError(t, err)

			_, err = c.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{Name: name, Content: testhelper.ExternalEmail(externalapi.EmailBroadcastContentInput{})})
			refused := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
			assert.Equal(t, "name must not be empty", refused.Detail.Value)

			after, err := env.DB.Broadcast.Query().Where(broadcast.WorkspaceID(fixtures.AcmeID)).Count(ctx)
			require.NoError(t, err)
			assert.Equal(t, before, after, "nothing is stored")
		})
		t.Run("update "+name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := context.Background()
			c := env.ExternalScoped(t, authorScopes...)

			_, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Name: externalapi.NewOptString(name)},
				externalapi.BroadcastsUpdateParams{ID: draftBroadcast})
			refused := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
			assert.Equal(t, "name must not be empty", refused.Detail.Value)
			assert.Equal(t, fixtures.BroadcastDraftName, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Name)
		})
	}
}

// A provider failure on a test send is a fault of the send, not a refusal of the request: a 500
// that says nothing of the cause, and the broadcast stays a draft.
func TestExternalBroadcastTestSendReportsAFailedSend(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, authorScopes...)
	env.CustomerMail.SetErr(errors.New("smtp unavailable"))

	err := c.BroadcastsTestSend(ctx, &externalapi.TestSendBroadcastInput{Email: externalapi.NewOptEmailAddress("qa@test.dev")},
		externalapi.BroadcastsTestSendParams{ID: draftBroadcast})
	require.ErrorContains(t, err, "unexpected status code: 500")
	assert.Empty(t, env.CustomerMail.Messages())
	assert.Equal(t, broadcast.StatusDraft, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Status)
}

// The report carries the reason a broadcast is held.
func TestExternalBroadcastReportCarriesTheHoldReason(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, authorScopes...)
	env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetHoldReason(holdreason.WorkspaceSuspended).ExecX(ctx)

	report, err := c.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.Equal(t, externalapi.HoldReasonWorkspaceSuspended, report.HoldReason.Value)
}
