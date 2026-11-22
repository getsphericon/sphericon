package automations_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

type httpResponse struct {
	status     int
	body       string
	retryAfter string
	returned   func()
}

func (r httpResponse) Do(*http.Request) (*http.Response, error) {
	if r.returned != nil {
		r.returned()
	}
	return &http.Response{StatusCode: r.status, Body: io.NopCloser(strings.NewReader(r.body)), Header: http.Header{"Retry-After": []string{r.retryAfter}}}, nil
}

func noFence(ctx context.Context, _ *ent.Scoped, _ int64, effect func(context.Context) error) error {
	return effect(ctx)
}

func executionNow() time.Time {
	return time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
}

func executionAttempt() httpcall.Attempt {
	now := executionNow()
	return httpcall.Attempt{Number: 1, Limit: 3, CreatedAt: now}
}

// The fixture HTTP step is its version's last: a call that succeeds leaves it and completes the
// Enrollment in the same move, on that Step, with nothing queued after it.
func TestHTTPExecutionOnTheLastStepCompletesTheEnrollment(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), noFence, env.Cipher, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: env.StepIDAt(t, runID, 0), Attempt: executionAttempt()})

	require.NoError(t, err)
	assert.Equal(t, httpcall.Execution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusCompleted, run.Status)
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(run.CurrentStepID))
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionKeepsTemporaryFailuresActiveUntilRecovery(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), noFence, env.Cipher, httpResponse{status: http.StatusServiceUnavailable}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: env.StepIDAt(t, runID, 0), Attempt: executionAttempt()})

	require.NoError(t, err)
	require.Error(t, decision.Retry)
	require.NoError(t, decision.Cancel)
	assert.Zero(t, decision.Wait)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(run.CurrentStepID))
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)

	attempt := executionAttempt()
	attempt.Number = 2
	decision, err = automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), noFence, env.Cipher, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: env.StepIDAt(t, runID, 0), Attempt: attempt})
	require.NoError(t, err)
	assert.Equal(t, httpcall.Execution{}, decision)
	run, err = s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusCompleted, run.Status, "the recovered call left the last Step")
	queued = env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionRecordsPermanentRefusalsAndCancels(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), noFence, env.Cipher, httpResponse{status: http.StatusNotFound, body: "gone"}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: env.StepIDAt(t, runID, 0), Attempt: executionAttempt()})

	require.NoError(t, err)
	require.Error(t, decision.Cancel)
	require.NoError(t, decision.Retry)
	assert.Zero(t, decision.Wait)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusFailed, run.Status)
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(run.CurrentStepID))
	require.NotNil(t, run.FailureKind)
	assert.Equal(t, automationrun.FailureKindHTTPStatus, *run.FailureKind)
	require.NotNil(t, run.FailureStatus)
	assert.Equal(t, http.StatusNotFound, *run.FailureStatus)
	require.NotNil(t, run.FailureExcerpt)
	assert.Equal(t, "gone", *run.FailureExcerpt)
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionRecordsTheExhaustedFailureAndReturnsItsRetry(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)
	attempt := executionAttempt()
	attempt.Number = attempt.Limit

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), noFence, env.Cipher, httpResponse{status: http.StatusServiceUnavailable}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: env.StepIDAt(t, runID, 0), Attempt: attempt})

	require.NoError(t, err)
	require.Error(t, decision.Retry)
	require.NoError(t, decision.Cancel)
	assert.Zero(t, decision.Wait)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusFailed, run.Status)
	require.NotNil(t, run.FailureStatus)
	assert.Equal(t, http.StatusServiceUnavailable, *run.FailureStatus)
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionRateLimitWaitsWithTheEnrollmentStillActive(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), noFence, env.Cipher, httpResponse{status: http.StatusTooManyRequests, retryAfter: "120"}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: env.StepIDAt(t, runID, 0), Attempt: executionAttempt()})

	require.NoError(t, err)
	assert.Equal(t, httpcall.Execution{Wait: 120 * time.Second}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(run.CurrentStepID))
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionCancelsAnInvalidRenderedBody(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	version, err := s.AutomationVersion().Get(t.Context(), fixtures.AutomationHTTPExecutionVersionID)
	require.NoError(t, err)
	call := version.Steps.Steps[0].Payload.(automationstep.HTTP)
	call.Body = `{"n": {{ first_name | default: 1 }}}`
	version.Steps.Steps[0].Payload = call
	err = s.AutomationVersion().UpdateOneID(version.ID).SetSteps(version.Steps).Exec(t.Context())
	require.NoError(t, err)
	runID := int64(fixtures.AutomationRunHTTPPendingID)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), noFence, env.Cipher, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: env.StepIDAt(t, runID, 0), Attempt: executionAttempt()})

	require.NoError(t, err)
	require.Error(t, decision.Cancel)
	require.NoError(t, decision.Retry)
	assert.Zero(t, decision.Wait)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusFailed, run.Status)
	require.NotNil(t, run.FailureKind)
	assert.Equal(t, automationrun.FailureKindInvalidBody, *run.FailureKind)
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionKeepsTheEnrollmentsPinnedVersionAfterPublish(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	a := testhelper.Module[*automations.Module](env)
	replaced := steps(automationstep.ApplyTag{Tag: "replaced-http"})
	_, err := a.Update(t.Context(), s, fixtures.AutomationHTTPExecutionID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHTTPExecutionID), Steps: replaced})
	require.NoError(t, err)
	_, err = a.Publish(t.Context(), s, fixtures.AutomationHTTPExecutionID, testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHTTPExecutionID))
	require.NoError(t, err)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), noFence, env.Cipher, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: env.StepIDAt(t, runID, 0), Attempt: executionAttempt()})

	require.NoError(t, err)
	assert.Equal(t, httpcall.Execution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusCompleted, run.Status, "the original HTTP Step still ran")
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(run.CurrentStepID))
	assert.Equal(t, int64(fixtures.AutomationHTTPExecutionVersionID), run.VersionID)
}

func TestHTTPExecutionCannotAdvanceWithoutItsNextQueuedStep(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)
	env.Bus.UseJobs(nil)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), noFence, env.Cipher, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: env.StepIDAt(t, runID, 0), Attempt: executionAttempt()})

	require.ErrorIs(t, err, events.ErrNoJobQueue)
	assert.Equal(t, httpcall.Execution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(run.CurrentStepID), "the move rolled back with its missing job")
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionIgnoresAnExitedEnrollment(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)
	pending, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	_, err = env.ExitEnrollment(t.Context(), s, pending.AutomationID, pending.ContactID)
	require.NoError(t, err)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), noFence, env.Cipher, httpResponse{status: http.StatusNotFound}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: env.StepIDAt(t, runID, 0), Attempt: executionAttempt()})

	require.NoError(t, err)
	assert.Equal(t, httpcall.Execution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusExited, run.Status)
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(run.CurrentStepID))
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionIgnoresAJobForAnotherStep(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), noFence, env.Cipher, httpResponse{status: http.StatusNotFound}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: env.StepIDAt(t, runID, 1), Attempt: executionAttempt()})

	require.NoError(t, err)
	assert.Equal(t, httpcall.Execution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(run.CurrentStepID))
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionIgnoresAnAlreadyAdvancedStep(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)
	_, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), noFence, env.Cipher, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: env.StepIDAt(t, runID, 0), Attempt: executionAttempt()})
	require.NoError(t, err)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), noFence, env.Cipher, httpResponse{status: http.StatusNotFound}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: env.StepIDAt(t, runID, 0), Attempt: executionAttempt()})

	require.NoError(t, err)
	assert.Equal(t, httpcall.Execution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusCompleted, run.Status, "the late attempt fails nothing")
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(run.CurrentStepID))
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionIgnoresErasedWork(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)
	step := env.StepIDAt(t, runID, 0)
	env.Feed(t, func(tx *ent.Client) error {
		return tx.Scoped(fixtures.AcmeID).Contact().DeleteOneID(fixtures.ContactAliceID).Exec(t.Context())
	})

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), noFence, env.Cipher, httpResponse{status: http.StatusNotFound}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: step, Attempt: executionAttempt()})

	require.NoError(t, err)
	assert.Equal(t, httpcall.Execution{}, decision)
	_, err = s.AutomationRun().Get(t.Context(), runID)
	assert.True(t, ent.IsNotFound(err))
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionStopsWhenTheResponseCrossesTheRateLimitHorizon(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	attempt := executionAttempt()
	now := attempt.CreatedAt.Add(webhook.HookRetryHorizon - time.Second)
	response := httpResponse{status: http.StatusTooManyRequests, returned: func() { now = now.Add(2 * time.Second) }}

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), noFence, env.Cipher, response, func() time.Time { return now }).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: fixtures.AutomationRunHTTPPendingID, Step: env.StepIDAt(t, fixtures.AutomationRunHTTPPendingID, 0), Attempt: attempt})

	require.NoError(t, err)
	require.Error(t, decision.Cancel)
	assert.Zero(t, decision.Wait)
	run, err := s.AutomationRun().Get(t.Context(), fixtures.AutomationRunHTTPPendingID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusFailed, run.Status)
	require.NotNil(t, run.FailureStatus)
	assert.Equal(t, http.StatusTooManyRequests, *run.FailureStatus)
}
