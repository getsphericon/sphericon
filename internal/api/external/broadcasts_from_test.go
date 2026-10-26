package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The From of a Broadcast is a verified Sending domain of the Workspace (ADR 0010): the external
// API, and so MCP, refuses another with 422 at save time, and an update that leaves the From
// alone is not re-checked, so a draft whose domain lost verification stays editable.
func TestExternalBroadcastFromEmailMustBeAVerifiedSendingDomain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "broadcasts:write")

	create := func(from string) externalapi.BroadcastsCreateRes {
		res, err := c.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{
			Name: "from check", FromEmail: externalapi.NewOptEmailAddress(externalapi.EmailAddress(from)),
		})
		require.NoError(t, err)
		return res
	}
	assert.IsType(t, &externalapi.BroadcastResource{}, create("news@"+fixtures.SendingDomainVerifiedDomain))
	assert.IsType(t, &externalapi.BroadcastsCreateUnprocessableEntity{}, create("news@"+fixtures.SendingDomainUnverifiedDomain))
	assert.IsType(t, &externalapi.BroadcastsCreateUnprocessableEntity{}, create("news@"+fixtures.SendingDomainGlobexDomain))

	refused, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{
		FromEmail: externalapi.NewOptEmailAddress("news@" + fixtures.SendingDomainUnverifiedDomain),
	}, externalapi.BroadcastsUpdateParams{ID: fixtures.BroadcastDraftID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsUpdateUnprocessableEntity{}, refused)

	edited, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{
		Subject: externalapi.NewOptString("New subject"),
	}, externalapi.BroadcastsUpdateParams{ID: fixtures.BroadcastDraftFromLostDomainID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastResource{}, edited)
}
