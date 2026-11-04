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

func TestExternalRulesDeleteRemovesARuleWithOrWithoutFirings(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "automations:read", "automations:write")

	for _, rule := range []int64{fixtures.RuleTagClickedID, fixtures.RuleInactiveID} {
		id := entityIDString(rule)
		deleted, err := c.RulesDelete(ctx, externalapi.RulesDeleteParams{ID: id})
		require.NoError(t, err)
		assert.IsType(t, &externalapi.RulesDeleteNoContent{}, deleted)

		gone, err := c.RulesGet(ctx, externalapi.RulesGetParams{ID: id})
		require.NoError(t, err)
		assert.IsType(t, &externalapi.RulesGetNotFound{}, gone)
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
