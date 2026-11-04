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

// Delete discards a Rule whether or not it has fired (ADR 0100): each is still read by id with
// discardedAt set, and a write to it is not found.
func TestExternalRulesDeleteKeepsARuleWithOrWithoutFirings(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "automations:read", "automations:write")

	for _, rule := range []int64{fixtures.RuleTagClickedID, fixtures.RuleInactiveID} {
		id := entityIDString(rule)
		deleted, err := c.RulesDelete(ctx, externalapi.RulesDeleteParams{ID: id})
		require.NoError(t, err)
		assert.IsType(t, &externalapi.RulesDeleteNoContent{}, deleted)

		kept, err := c.RulesGet(ctx, externalapi.RulesGetParams{ID: id})
		require.NoError(t, err)
		res, ok := kept.(*externalapi.RuleResource)
		require.Truef(t, ok, "got %T", kept)
		_, discardedAt := res.DiscardedAt.Get()
		assert.True(t, discardedAt, "the Rule is kept, marked discarded")
		renamed, err := c.RulesUpdate(ctx, &externalapi.UpdateRuleInput{Name: externalapi.NewOptString("renamed")},
			externalapi.RulesUpdateParams{ID: id})
		require.NoError(t, err)
		assert.IsType(t, &externalapi.RulesUpdateNotFound{}, renamed, "a deleted Rule changes no more")
	}
}

func TestExternalRulesDeleteNeedsTheAutomationsWriteScope(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	reader := env.ExternalScoped(t, "automations:read")

	del, err := reader.RulesDelete(ctx, externalapi.RulesDeleteParams{ID: entityIDString(fixtures.RuleInactiveID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.RulesDeleteForbidden{}, del)
}
