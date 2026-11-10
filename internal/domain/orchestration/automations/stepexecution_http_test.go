package automations_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
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

func TestHTTPExecutionAdvancesTheEnrollmentAndQueuesItsNextStep(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), noFence, env.Cipher, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: 0, Attempt: executionAttempt()})

	require.NoError(t, err)
	assert.Equal(t, automations.HTTPExecution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, 1, run.CurrentStep)
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Equal(t, []map[string]any{{"run_id": float64(runID), "step": float64(1), jobkind.WorkspaceArg: float64(run.WorkspaceID)}}, queued)
}

func TestHTTPExecutionKeepsTemporaryFailuresActiveUntilRecovery(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), noFence, env.Cipher, httpResponse{status: http.StatusServiceUnavailable}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: 0, Attempt: executionAttempt()})

	require.NoError(t, err)
	require.Error(t, decision.Retry)
	require.NoError(t, decision.Cancel)
	assert.Zero(t, decision.Wait)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, 0, run.CurrentStep)
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)

	attempt := executionAttempt()
	attempt.Number = 2
	decision, err = automations.NewStepExecution(env.Bus, env.OutboundModule(), noFence, env.Cipher, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: 0, Attempt: attempt})
	require.NoError(t, err)
	assert.Equal(t, automations.HTTPExecution{}, decision)
	run, err = s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, 1, run.CurrentStep)
	queued = env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Equal(t, []map[string]any{{"run_id": float64(runID), "step": float64(1), jobkind.WorkspaceArg: float64(run.WorkspaceID)}}, queued)
}

func TestHTTPExecutionRecordsPermanentRefusalsAndCancels(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), noFence, env.Cipher, httpResponse{status: http.StatusNotFound, body: "gone"}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: 0, Attempt: executionAttempt()})

	require.NoError(t, err)
	require.Error(t, decision.Cancel)
	require.NoError(t, decision.Retry)
	assert.Zero(t, decision.Wait)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusFailed, run.Status)
	assert.Equal(t, 0, run.CurrentStep)
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

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), noFence, env.Cipher, httpResponse{status: http.StatusServiceUnavailable}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: 0, Attempt: attempt})

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

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), noFence, env.Cipher, httpResponse{status: http.StatusTooManyRequests, retryAfter: "120"}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: 0, Attempt: executionAttempt()})

	require.NoError(t, err)
	assert.Equal(t, automations.HTTPExecution{Wait: 120 * time.Second}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, 0, run.CurrentStep)
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionCancelsAnInvalidRenderedBody(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	version, err := s.AutomationVersion().Get(t.Context(), fixtures.AutomationHTTPExecutionVersionID)
	require.NoError(t, err)
	steps, err := automationstep.Decode(version.Steps)
	require.NoError(t, err)
	steps[0].Body = `{"n": {{ first_name | default: 1 }}}`
	stored, err := json.Marshal(steps)
	require.NoError(t, err)
	err = s.AutomationVersion().UpdateOneID(version.ID).SetSteps(string(stored)).Exec(t.Context())
	require.NoError(t, err)
	runID := int64(fixtures.AutomationRunHTTPPendingID)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), noFence, env.Cipher, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: 0, Attempt: executionAttempt()})

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
	a := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	steps := []automationstep.Step{{Type: automationstep.ApplyTag, Tag: "replaced-http"}}
	_, err := a.Update(t.Context(), s, fixtures.AutomationHTTPExecutionID, automations.UpdateInput{Steps: &steps})
	require.NoError(t, err)
	_, err = a.Publish(t.Context(), s, fixtures.AutomationHTTPExecutionID)
	require.NoError(t, err)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), noFence, env.Cipher, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: 0, Attempt: executionAttempt()})

	require.NoError(t, err)
	assert.Equal(t, automations.HTTPExecution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, 1, run.CurrentStep, "the original HTTP Step still ran")
	assert.Equal(t, int64(fixtures.AutomationHTTPExecutionVersionID), run.VersionID)
}

func TestHTTPExecutionCannotAdvanceWithoutItsNextQueuedStep(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)
	env.Bus.UseJobs(nil)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), noFence, env.Cipher, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: 0, Attempt: executionAttempt()})

	require.ErrorIs(t, err, events.ErrNoJobQueue)
	assert.Equal(t, automations.HTTPExecution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, 0, run.CurrentStep, "the move rolled back with its missing job")
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionIgnoresAnExitedEnrollment(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)
	pending, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	_, err = automations.Exit(t.Context(), s, env.Publisher(), pending.AutomationID, pending.ContactID, time.Now())
	require.NoError(t, err)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), noFence, env.Cipher, httpResponse{status: http.StatusNotFound}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: 0, Attempt: executionAttempt()})

	require.NoError(t, err)
	assert.Equal(t, automations.HTTPExecution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusExited, run.Status)
	assert.Equal(t, 0, run.CurrentStep)
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionIgnoresAJobForAnotherStep(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), noFence, env.Cipher, httpResponse{status: http.StatusNotFound}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: 1, Attempt: executionAttempt()})

	require.NoError(t, err)
	assert.Equal(t, automations.HTTPExecution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, 0, run.CurrentStep)
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionIgnoresAnAlreadyAdvancedStep(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)
	_, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), noFence, env.Cipher, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: 0, Attempt: executionAttempt()})
	require.NoError(t, err)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), noFence, env.Cipher, httpResponse{status: http.StatusNotFound}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: 0, Attempt: executionAttempt()})

	require.NoError(t, err)
	assert.Equal(t, automations.HTTPExecution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, 1, run.CurrentStep)
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Equal(t, []map[string]any{{"run_id": float64(runID), "step": float64(1), jobkind.WorkspaceArg: float64(run.WorkspaceID)}}, queued)
}

func TestHTTPExecutionIgnoresErasedWork(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)
	err := env.FeedDB.Scoped(fixtures.AcmeID).Contact().DeleteOneID(fixtures.ContactAliceID).Exec(t.Context())
	require.NoError(t, err)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), noFence, env.Cipher, httpResponse{status: http.StatusNotFound}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: 0, Attempt: executionAttempt()})

	require.NoError(t, err)
	assert.Equal(t, automations.HTTPExecution{}, decision)
	_, err = s.AutomationRun().Get(t.Context(), runID)
	assert.True(t, ent.IsNotFound(err))
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionCancelsADamagedStoredDefinition(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)
	err := s.AutomationVersion().UpdateOneID(fixtures.AutomationHTTPExecutionVersionID).SetSteps("[").Exec(t.Context())
	require.NoError(t, err)

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), noFence, env.Cipher, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: runID, Step: 0, Attempt: executionAttempt()})

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

func TestHTTPExecutionStopsWhenTheResponseCrossesTheRateLimitHorizon(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	attempt := executionAttempt()
	now := attempt.CreatedAt.Add(webhook.HookRetryHorizon - time.Second)
	response := httpResponse{status: http.StatusTooManyRequests, returned: func() { now = now.Add(2 * time.Second) }}

	decision, err := automations.NewStepExecution(env.Bus, env.OutboundModule(), noFence, env.Cipher, response, func() time.Time { return now }).ExecuteHTTP(t.Context(), s, automations.ExecuteHTTPInput{RunID: fixtures.AutomationRunHTTPPendingID, Step: 0, Attempt: attempt})

	require.NoError(t, err)
	require.Error(t, decision.Cancel)
	assert.Zero(t, decision.Wait)
	run, err := s.AutomationRun().Get(t.Context(), fixtures.AutomationRunHTTPPendingID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusFailed, run.Status)
	require.NotNil(t, run.FailureStatus)
	assert.Equal(t, http.StatusTooManyRequests, *run.FailureStatus)
}
