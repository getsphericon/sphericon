package external_test

import (
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Contact's skipped entries into an Automation are read with automations:read, each with its
// reason and the Event that tried (ADR 0127).
func TestExternalSkippedEntriesOfAContactAreReadWithAutomationsRead(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead)

	listed, err := c.SkippedEntriesList(t.Context(), externalapi.SkippedEntriesListParams{
		AutomationId: entityIDString(fixtures.AutomationWelcomeSeriesID),
		ContactId:    externalapi.NewOptEntityId(entityIDString(fixtures.ContactLiamID)),
	})

	require.NoError(t, err)
	ids := lo.Map(listed.Items, func(e externalapi.SkippedEntryResource, _ int) externalapi.EntityId { return e.ID })
	assert.Equal(t, []externalapi.EntityId{entityIDString(fixtures.SkippedEntryWelcomeLiamID), entityIDString(fixtures.SkippedEntryExpiredID)}, ids,
		"Liam's, newest first; Emma's is not his")
	newest := listed.Items[0]
	assert.Equal(t, externalapi.SkippedEntryReasonOnce, newest.Reason)
	assert.Equal(t, entityIDString(fixtures.ContactLiamID), newest.ContactId)
	assert.Equal(t, "fixture:skipped-welcome-liam", newest.EventSourceId.Or(""))
	assert.False(t, newest.FiringId.IsSet())
}

func TestExternalSkippedEntriesNeedAutomationsReadAndStayInTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	denied := env.ExternalScoped(t, apitokens.ScopeEnrollmentsRead)
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead)

	_, err := denied.SkippedEntriesList(ctx, externalapi.SkippedEntriesListParams{AutomationId: entityIDString(fixtures.AutomationWelcomeSeriesID)})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	_, err = c.SkippedEntriesList(ctx, externalapi.SkippedEntriesListParams{AutomationId: entityIDString(fixtures.AutomationGlobexID)})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
}
