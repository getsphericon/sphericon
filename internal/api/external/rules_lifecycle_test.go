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

func TestExternalRulesArchiveUnarchiveAndDelete(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "automations:read", "automations:write")

	archived, err := c.RulesArchive(ctx, externalapi.RulesArchiveParams{ID: entityIDString(fixtures.RuleTagClickedID)})
	require.NoError(t, err)
	res, ok := archived.(*externalapi.RuleResource)
	require.Truef(t, ok, "got %T", archived)
	_, isArchived := res.ArchivedAt.Get()
	assert.True(t, isArchived)
	assert.Equal(t, externalapi.RuleStatusDraft, res.Status)

	list, err := c.RulesList(ctx, externalapi.RulesListParams{Archived: externalapi.NewOptBool(true)})
	require.NoError(t, err)
	assert.Len(t, list.(*externalapi.RulesListOK).Items, 1)

	restored, err := c.RulesUnarchive(ctx, externalapi.RulesUnarchiveParams{ID: entityIDString(fixtures.RuleTagClickedID)})
	require.NoError(t, err)
	back, ok := restored.(*externalapi.RuleResource)
	require.Truef(t, ok, "got %T", restored)
	_, isArchived = back.ArchivedAt.Get()
	assert.False(t, isArchived)

	refused, err := c.RulesDelete(ctx, externalapi.RulesDeleteParams{ID: entityIDString(fixtures.RuleTagClickedID)})
	require.NoError(t, err)
	problem, ok := refused.(*externalapi.InUseProblemDetails)
	require.Truef(t, ok, "got %T", refused)
	require.Len(t, problem.UsedBy, 1)
	assert.Equal(t, externalapi.UsedByKindFiring, problem.UsedBy[0].Kind)

	deleted, err := c.RulesDelete(ctx, externalapi.RulesDeleteParams{ID: entityIDString(fixtures.RuleDraftID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.RulesDeleteNoContent{}, deleted)
}

func TestExternalRulesArchiveAndDeleteNeedTheAutomationsWriteScope(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	reader := env.ExternalScoped(t, "automations:read")

	archive, err := reader.RulesArchive(ctx, externalapi.RulesArchiveParams{ID: entityIDString(fixtures.RuleDraftID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.RulesArchiveUnauthorized{}, archive)
	del, err := reader.RulesDelete(ctx, externalapi.RulesDeleteParams{ID: entityIDString(fixtures.RuleDraftID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.RulesDeleteUnauthorized{}, del)
}
