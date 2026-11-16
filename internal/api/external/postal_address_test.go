package external_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An agent reads the Workspace's postal address before it arms marketing email (ADR 0156): it
// cannot set one, so it sees the gap and asks a person.
func TestExternalWorkspaceCarriesItsPostalAddress(t *testing.T) {
	env := testhelper.Setup(t)

	ws := workspaceOf(t, env)

	assert.Equal(t, "100 Market Street\nSpringfield", ws.PostalAddress)
}

func TestExternalSchedulingAnEmailBroadcastWithoutAPostalAddressIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	env.ClearPostalAddress(t, fixtures.AcmeID)
	c := env.ExternalScoped(t, apitokens.ScopeBroadcastsSend)

	_, err := c.BroadcastsSchedule(t.Context(), &externalapi.ScheduleBroadcastInput{
		Revision: externalapi.Revision(env.BroadcastRevision(t.Context(), t, int64(fixtures.BroadcastDraftID))), ScheduledAt: externalapi.Timestamp(time.Now().Add(time.Hour)),
	}, externalapi.BroadcastsScheduleParams{ID: draftBroadcast})

	refused := testhelper.ExternalRefusal(t, err, http.StatusConflict)
	assert.Equal(t, externalapi.ProblemCodePostalAddressRequired, refused.Code.Value)
	read := workspaceOf(t, env)
	assert.Empty(t, read.PostalAddress, "the agent sees the gap the refusal names")
}
