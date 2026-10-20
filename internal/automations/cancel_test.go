package automations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// cancelFor runs CancelPending for a Contact the way Erasure does, in a purging transaction.
func cancelFor(t *testing.T, env *testhelper.TestEnv, contactID int64) error {
	t.Helper()
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.InitechID)
	return env.Bus.WithinScopedPurgeTx(ctx, s, func(ts *ent.Scoped, pub events.PurgingPublisher) error {
		return automations.CancelPending(ctx, ts, contactID, pub)
	})
}

func runIDsOf(t *testing.T, env *testhelper.TestEnv, kind string) []any {
	t.Helper()
	var ids []any
	for _, args := range env.JobsOf(t, kind) {
		ids = append(ids, args["run_id"])
	}
	return ids
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
