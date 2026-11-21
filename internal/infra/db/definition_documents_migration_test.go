package db_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/ruleaction"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The definition documents become jsonb of their Go types (ADR 0169): a Condition with tests reads
// back as it was, ” and a missing Condition of a Segment or a Rule version become the empty group,
// which matches every Contact as they did, an armed Broadcast's ” copy becomes it too, and a
// Broadcast to all Contacts keeps no Condition.
func TestTheDefinitionDocumentsMigrationKeepsEveryConditionAndSpellsEmptyAsTheEmptyGroup(t *testing.T) {
	sqlDB := testhelper.SchemaBefore(t, "definition_documents_jsonb", filepath.Join("testdata", "definition_documents_jsonb"))

	_, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)
	client := entclient.Core.New(sqlDB)
	ctx := t.Context()

	assert.JSONEq(t, `{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"plan","test":{"kind":"text","operator":"eq","value":"pro"}}]}`,
		testhelper.Document(t, client.Segment.GetX(ctx, 1).Condition))
	assert.Equal(t, conditiontree.Segment{Group: conditiontree.Empty()}, client.Segment.GetX(ctx, 2).Condition, "''")
	assert.Equal(t, conditiontree.Segment{Group: conditiontree.Empty()}, client.Segment.GetX(ctx, 3).Condition, "no Condition")
	first := client.RuleVersion.GetX(ctx, 1)
	assert.Equal(t, conditiontree.Rule{Group: conditiontree.Empty()}, first.Condition, "no Condition")
	assert.Equal(t, []ruleaction.Action{{Type: ruleaction.ApplyTag, Tag: "clicked"}}, first.Actions)
	assert.Equal(t, conditiontree.Rule{Group: conditiontree.Empty()}, client.RuleVersion.GetX(ctx, 2).Condition, "''")
	assert.Equal(t, &conditiontree.Segment{Group: conditiontree.Empty()}, client.Broadcast.GetX(ctx, 1).Condition, "''")
	assert.Nil(t, client.Broadcast.GetX(ctx, 2).Condition)
}
