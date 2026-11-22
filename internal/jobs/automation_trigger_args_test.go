package jobs_test

import (
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A trigger evaluation names its Event: when it occurred and the id its row is stored under. One
// without either fails, as a job whose args do not decode does, and enrolls nobody.
func TestATriggerJobWithoutItsEventFails(t *testing.T) {
	env := testhelper.Setup(t)
	worker := jobs.NewEvaluateTriggerWorker(env.DB, testhelper.Module[*automations.StepExecution](env))
	contactID := int64(fixtures.ContactPaidBeforeEnrolledID)
	withoutTime := jobs.EvaluateTriggerArgs{ContactID: contactID, Action: "payment_failed", SourceID: "evt-without-time"}
	withoutSource := jobs.EvaluateTriggerArgs{ContactID: contactID, Action: "payment_failed", OccurredAt: time.Now()}

	for _, args := range []jobs.EvaluateTriggerArgs{withoutTime, withoutSource} {
		args.WorkspaceID = fixtures.AcmeID
		err := worker.Work(t.Context(), &river.Job[jobs.EvaluateTriggerArgs]{Args: args})

		require.ErrorContains(t, err, "occurred_at and source_id are required")
	}
	assert.Empty(t, enrollmentsOf(t, env, fixtures.AutomationPaymentRecoveryID, contactID))
}
