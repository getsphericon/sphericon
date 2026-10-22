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

// A dry run only reads, so the automations:read scope is enough; it writes no Firing.
func TestExternalRulesDryRunNeedsOnlyTheReadScopeAndWritesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	reader := env.ExternalScoped(t, "automations:read")
	firings := env.DB.RuleFiring.Query().CountX(ctx)
	params := externalapi.RulesDryRunParams{ID: entityIDString(fixtures.RuleTagClickedID)}
	body := &externalapi.DryRunRuleInput{EventId: entityIDString(fixtures.EventRuleClickLiamID)}

	out, err := reader.RulesDryRun(ctx, body, params)

	require.NoError(t, err)
	res, ok := out.(*externalapi.RuleDryRunResource)
	require.Truef(t, ok, "got %T", out)
	assert.Equal(t, externalapi.RuleDryRunOutcomeWouldFire, res.Outcome)
	require.Len(t, res.Actions, 1)
	assert.Equal(t, "apply_tag", res.Actions[0].Type)
	assert.Equal(t, firings, env.DB.RuleFiring.Query().CountX(ctx))

	denied, err := env.ExternalScoped(t, "contacts:read").RulesDryRun(ctx, body, params)
	require.NoError(t, err)
	assert.IsType(t, &externalapi.RulesDryRunUnauthorized{}, denied)
}
