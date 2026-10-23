package automations_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// cancelFor runs CancelPending for a Contact the way Erasure does.
func cancelFor(t *testing.T, env *testhelper.TestEnv, contactID int64) error {
	t.Helper()
	return env.CancelPendingFor(t, contactID, automations.CancelPending)
}

func runIDsOf(t *testing.T, env *testhelper.TestEnv, kind string) []any {
	t.Helper()
	return lo.Map(env.JobsOf(t, kind), func(args map[string]any, _ int) any { return args["run_id"] })
}

func TestCancelPendingRemovesTheContactsQueuedCallsAndKeepsOthers(t *testing.T) {
	env := testhelper.Setup(t)
	for _, id := range []int64{
		fixtures.AutomationRunErasableID, fixtures.AutomationRunErasableHTTPFailedID,
		fixtures.AutomationRunBystanderHTTPFailedID,
	} {
		env.EnqueueJob(t, jobs.RunStepArgs{RunID: id})
		env.EnqueueJob(t, jobs.RunHTTPStepArgs{RunID: id})
	}
	env.EnqueueJob(t, jobs.EvaluateTriggerArgs{WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactErasableID, Action: "x"})
	env.EnqueueJob(t, jobs.EvaluateTriggerArgs{WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactBystanderID, Action: "x"})

	require.NoError(t, cancelFor(t, env, fixtures.ContactErasableID))

	bystander := []any{float64(fixtures.AutomationRunBystanderHTTPFailedID)}
	assert.Equal(t, bystander, runIDsOf(t, env, jobkind.RunStep.Kind))
	assert.Equal(t, bystander, runIDsOf(t, env, jobkind.HTTPStep.Kind))
	left := env.JobsOf(t, jobkind.EvaluateTrigger.Kind)
	require.Len(t, left, 1)
	assert.EqualValues(t, fixtures.ContactBystanderID, left[0]["contact_id"])
}
