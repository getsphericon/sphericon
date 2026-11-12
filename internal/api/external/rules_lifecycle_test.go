package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Delete discards a Rule whether or not it has fired (ADR 0100): each is still read by id with
// discardedAt set, and a write to it is not found.
func TestExternalRulesDeleteKeepsARuleWithOrWithoutFirings(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite)

	for _, rule := range []int64{fixtures.RuleTagClickedID, fixtures.RuleInactiveID} {
		id := entityIDString(rule)
		err := c.RulesDelete(ctx, externalapi.RulesDeleteParams{ID: id})
		require.NoError(t, err)

		kept, err := c.RulesGet(ctx, externalapi.RulesGetParams{ID: id})
		require.NoError(t, err)
		res := kept
		_, discardedAt := res.DiscardedAt.Get()
		assert.True(t, discardedAt, "the Rule is kept, marked discarded")
		_, err = c.RulesUpdate(ctx, &externalapi.UpdateRuleInput{Revision: externalapi.Revision(env.RuleRevision(ctx, t, int64(id))), Name: externalapi.NewOptName("renamed")},
			externalapi.RulesUpdateParams{ID: id})
		testhelper.ExternalRefusal(t, err, http.StatusNotFound, "a deleted Rule changes no more")
	}
}

func TestExternalRulesDeleteNeedsTheAutomationsWriteScope(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	reader := env.ExternalScoped(t, apitokens.ScopeAutomationsRead)

	err := reader.RulesDelete(ctx, externalapi.RulesDeleteParams{ID: entityIDString(fixtures.RuleInactiveID)})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
}
