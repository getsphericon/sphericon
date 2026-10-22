package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An erased Contact leaves no Firing and no queued evaluation of an Event of theirs; the bystander's
// stay.
func TestExternalContactsEraseRemovesRuleFiringsAndQueuedEvaluations(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	env.EnqueueJob(t, jobs.EvaluateRulesArgs{WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactErasableID, Action: "rule_initech", SourceID: "evt-1"})
	env.EnqueueJob(t, jobs.EvaluateRulesArgs{WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactBystanderID, Action: "rule_initech", SourceID: "evt-2"})

	eraseErasable(t, env)

	erased, err := env.DB.RuleFiring.Query().Where(rulefiring.ContactID(fixtures.ContactErasableID)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, erased)
	kept, err := env.DB.RuleFiring.Query().Where(rulefiring.ID(fixtures.RuleFiringBystanderID)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, kept)
	queued := env.JobsOf(t, jobkind.RuleEvaluate.Kind)
	require.Len(t, queued, 1)
	assert.EqualValues(t, fixtures.ContactBystanderID, queued[0]["contact_id"])
}
