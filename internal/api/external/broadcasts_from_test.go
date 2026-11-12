package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The From of a Broadcast is a verified Sending domain of the Workspace (ADR 0010): the external
// API, and so MCP, refuses another with 422 at save time, and an update that leaves the From
// alone is not re-checked, so a draft whose domain lost verification stays editable.
func TestExternalBroadcastFromEmailMustBeAVerifiedSendingDomain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, apitokens.ScopeBroadcastsWrite)

	create := func(from string) error {
		_, err := c.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{
			Name:    "from check",
			Content: testhelper.ExternalEmail(externalapi.EmailBroadcastContentInput{FromEmail: externalapi.NewOptNilEmailAddress(externalapi.EmailAddress(from))}),
		})
		return err
	}
	require.NoError(t, create("news@"+fixtures.SendingDomainVerifiedDomain))
	testhelper.ExternalRefusal(t, create("news@"+fixtures.SendingDomainUnverifiedDomain), http.StatusUnprocessableEntity)
	testhelper.ExternalRefusal(t, create("news@"+fixtures.SendingDomainGlobexDomain), http.StatusUnprocessableEntity)

	_, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(fixtures.BroadcastDraftID))),
		Content: testhelper.ExternalEmailEdit(externalapi.EmailBroadcastContentInput{FromEmail: externalapi.NewOptNilEmailAddress("news@" + fixtures.SendingDomainUnverifiedDomain)}),
	}, externalapi.BroadcastsUpdateParams{ID: fixtures.BroadcastDraftID})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)

	_, err = c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(fixtures.BroadcastDraftFromLostDomainID))),
		Content: testhelper.ExternalEmailEdit(externalapi.EmailBroadcastContentInput{Subject: externalapi.NewOptString("New subject")}),
	}, externalapi.BroadcastsUpdateParams{ID: fixtures.BroadcastDraftFromLostDomainID})
	require.NoError(t, err)
}
