package stepping_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/automationsteps"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/webhook"
)

type httpResponse struct {
	status     int
	body       string
	retryAfter string
}

func (r httpResponse) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: r.status, Body: io.NopCloser(strings.NewReader(r.body)), Header: http.Header{"Retry-After": []string{r.retryAfter}}}, nil
}

func noFence(ctx context.Context, _ *ent.Scoped, _ int64, effect func(context.Context) error) error {
	return effect(ctx)
}

func httpEnrollment(t *testing.T, env *testhelper.TestEnv) int64 {
	t.Helper()
	s := env.DB.Scoped(fixtures.AcmeID)
	runID, enrolled, err := automations.Enroll(t.Context(), s, env.Publisher(), fixtures.AutomationNotifySlackID, fixtures.ContactAliceID)
	require.NoError(t, err)
	require.True(t, enrolled)
	return runID
}

func executionAttempt() httpcall.Attempt {
	now := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	return httpcall.Attempt{Number: 1, Limit: 3, CreatedAt: now, Now: now}
}

func TestHTTPExecutionAdvancesTheEnrollmentAndQueuesItsNextStep(t *testing.T) {
	env := testhelper.Setup(t)
	m := stepping.New(env.Bus, env.OutboundModule(), noFence)
	runID := httpEnrollment(t, env)
	s := env.DB.Scoped(fixtures.AcmeID)

	decision, err := m.ExecuteHTTP(t.Context(), s, runID, 0, executionAttempt(), env.Cipher, httpResponse{status: http.StatusOK})

	require.NoError(t, err)
	assert.Equal(t, stepping.HTTPExecution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, 1, run.CurrentStep)
	queued := env.JobsOf(t, stepping.RunStepArgs{}.Kind())
	assert.Equal(t, []map[string]any{{"run_id": float64(runID), "step": float64(1)}}, queued)
}

func TestHTTPExecutionKeepsTemporaryFailuresActiveUntilRecovery(t *testing.T) {
	env := testhelper.Setup(t)
	m := stepping.New(env.Bus, env.OutboundModule(), noFence)
	runID := httpEnrollment(t, env)
	s := env.DB.Scoped(fixtures.AcmeID)

	decision, err := m.ExecuteHTTP(t.Context(), s, runID, 0, executionAttempt(), env.Cipher, httpResponse{status: http.StatusServiceUnavailable})

	require.NoError(t, err)
	require.Error(t, decision.Retry)
	require.NoError(t, decision.Cancel)
	assert.Zero(t, decision.Wait)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, 0, run.CurrentStep)
	queued := env.JobsOf(t, stepping.RunStepArgs{}.Kind())
	assert.Empty(t, queued)

	attempt := executionAttempt()
	attempt.Number = 2
	decision, err = m.ExecuteHTTP(t.Context(), s, runID, 0, attempt, env.Cipher, httpResponse{status: http.StatusOK})
	require.NoError(t, err)
	assert.Equal(t, stepping.HTTPExecution{}, decision)
	run, err = s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, 1, run.CurrentStep)
	queued = env.JobsOf(t, stepping.RunStepArgs{}.Kind())
	assert.Equal(t, []map[string]any{{"run_id": float64(runID), "step": float64(1)}}, queued)
}

func TestHTTPExecutionRecordsPermanentRefusalsAndCancels(t *testing.T) {
	env := testhelper.Setup(t)
	m := stepping.New(env.Bus, env.OutboundModule(), noFence)
	runID := httpEnrollment(t, env)
	s := env.DB.Scoped(fixtures.AcmeID)

	decision, err := m.ExecuteHTTP(t.Context(), s, runID, 0, executionAttempt(), env.Cipher, httpResponse{status: http.StatusNotFound, body: "gone"})

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
	queued := env.JobsOf(t, stepping.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionRecordsTheExhaustedFailureAndReturnsItsRetry(t *testing.T) {
	env := testhelper.Setup(t)
	m := stepping.New(env.Bus, env.OutboundModule(), noFence)
	runID := httpEnrollment(t, env)
	s := env.DB.Scoped(fixtures.AcmeID)
	attempt := executionAttempt()
	attempt.Number = attempt.Limit

	decision, err := m.ExecuteHTTP(t.Context(), s, runID, 0, attempt, env.Cipher, httpResponse{status: http.StatusServiceUnavailable})

	require.NoError(t, err)
	require.Error(t, decision.Retry)
	require.NoError(t, decision.Cancel)
	assert.Zero(t, decision.Wait)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusFailed, run.Status)
	require.NotNil(t, run.FailureStatus)
	assert.Equal(t, http.StatusServiceUnavailable, *run.FailureStatus)
	queued := env.JobsOf(t, stepping.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionRateLimitWaitsWithoutSpendingItsLastAttempt(t *testing.T) {
	env := testhelper.Setup(t)
	m := stepping.New(env.Bus, env.OutboundModule(), noFence)
	runID := httpEnrollment(t, env)
	s := env.DB.Scoped(fixtures.AcmeID)
	attempt := executionAttempt()
	attempt.Number = attempt.Limit

	decision, err := m.ExecuteHTTP(t.Context(), s, runID, 0, attempt, env.Cipher, httpResponse{status: http.StatusTooManyRequests, retryAfter: "120"})

	require.NoError(t, err)
	assert.Equal(t, stepping.HTTPExecution{Wait: 120 * time.Second}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, 0, run.CurrentStep)
	queued := env.JobsOf(t, stepping.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionRateLimitFailsAtItsHorizon(t *testing.T) {
	env := testhelper.Setup(t)
	m := stepping.New(env.Bus, env.OutboundModule(), noFence)
	runID := httpEnrollment(t, env)
	s := env.DB.Scoped(fixtures.AcmeID)
	attempt := executionAttempt()
	attempt.Now = attempt.Now.Add(webhook.HookRetryHorizon)

	decision, err := m.ExecuteHTTP(t.Context(), s, runID, 0, attempt, env.Cipher, httpResponse{status: http.StatusTooManyRequests})

	require.NoError(t, err)
	require.Error(t, decision.Cancel)
	require.NoError(t, decision.Retry)
	assert.Zero(t, decision.Wait)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusFailed, run.Status)
	require.NotNil(t, run.FailureStatus)
	assert.Equal(t, http.StatusTooManyRequests, *run.FailureStatus)
}

func TestHTTPExecutionCancelsAnInvalidRenderedBody(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	a := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	steps := []automationsteps.Step{{Type: automationsteps.HTTP, URL: "https://hooks.example.com/notify", Body: `{"n": {{ first_name | default: 1 }}}`}}
	_, err := a.Update(t.Context(), s, fixtures.AutomationNotifySlackID, automations.UpdateInput{Steps: &steps})
	require.NoError(t, err)
	_, err = a.Publish(t.Context(), s, fixtures.AutomationNotifySlackID)
	require.NoError(t, err)
	runID := httpEnrollment(t, env)
	m := stepping.New(env.Bus, env.OutboundModule(), noFence)

	decision, err := m.ExecuteHTTP(t.Context(), s, runID, 0, executionAttempt(), env.Cipher, httpResponse{status: http.StatusOK})

	require.NoError(t, err)
	require.Error(t, decision.Cancel)
	require.NoError(t, decision.Retry)
	assert.Zero(t, decision.Wait)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusFailed, run.Status)
	require.NotNil(t, run.FailureKind)
	assert.Equal(t, automationrun.FailureKindInvalidBody, *run.FailureKind)
	queued := env.JobsOf(t, stepping.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionKeepsTheEnrollmentsPinnedVersionAfterPublish(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	runID := httpEnrollment(t, env)
	a := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	steps := []automationsteps.Step{{Type: automationsteps.ApplyTag, Tag: "replaced-http"}}
	_, err := a.Update(t.Context(), s, fixtures.AutomationNotifySlackID, automations.UpdateInput{Steps: &steps})
	require.NoError(t, err)
	_, err = a.Publish(t.Context(), s, fixtures.AutomationNotifySlackID)
	require.NoError(t, err)
	m := stepping.New(env.Bus, env.OutboundModule(), noFence)

	decision, err := m.ExecuteHTTP(t.Context(), s, runID, 0, executionAttempt(), env.Cipher, httpResponse{status: http.StatusOK})

	require.NoError(t, err)
	assert.Equal(t, stepping.HTTPExecution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, 1, run.CurrentStep, "the original HTTP Step still ran")
	assert.Equal(t, int64(fixtures.AutomationNotifySlackVersionID), run.VersionID)
}

func TestHTTPExecutionCannotAdvanceWithoutItsNextQueuedStep(t *testing.T) {
	env := testhelper.Setup(t)
	m := stepping.New(env.Bus, env.OutboundModule(), noFence)
	runID := httpEnrollment(t, env)
	s := env.DB.Scoped(fixtures.AcmeID)
	env.Bus.UseJobs(nil)

	decision, err := m.ExecuteHTTP(t.Context(), s, runID, 0, executionAttempt(), env.Cipher, httpResponse{status: http.StatusOK})

	require.ErrorIs(t, err, events.ErrNoJobQueue)
	assert.Equal(t, stepping.HTTPExecution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, 0, run.CurrentStep, "the move rolled back with its missing job")
	queued := env.JobsOf(t, stepping.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionIgnoresAnExitedEnrollment(t *testing.T) {
	env := testhelper.Setup(t)
	m := stepping.New(env.Bus, env.OutboundModule(), noFence)
	runID := httpEnrollment(t, env)
	s := env.DB.Scoped(fixtures.AcmeID)
	err := automations.ExitRun(t.Context(), s, env.Publisher(), runID, time.Now())
	require.NoError(t, err)

	decision, err := m.ExecuteHTTP(t.Context(), s, runID, 0, executionAttempt(), env.Cipher, httpResponse{status: http.StatusNotFound})

	require.NoError(t, err)
	assert.Equal(t, stepping.HTTPExecution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusExited, run.Status)
	assert.Equal(t, 0, run.CurrentStep)
	queued := env.JobsOf(t, stepping.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionIgnoresAJobForAnotherStep(t *testing.T) {
	env := testhelper.Setup(t)
	m := stepping.New(env.Bus, env.OutboundModule(), noFence)
	runID := httpEnrollment(t, env)
	s := env.DB.Scoped(fixtures.AcmeID)

	decision, err := m.ExecuteHTTP(t.Context(), s, runID, 1, executionAttempt(), env.Cipher, httpResponse{status: http.StatusNotFound})

	require.NoError(t, err)
	assert.Equal(t, stepping.HTTPExecution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, 0, run.CurrentStep)
	queued := env.JobsOf(t, stepping.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionIgnoresAnAlreadyAdvancedStep(t *testing.T) {
	env := testhelper.Setup(t)
	m := stepping.New(env.Bus, env.OutboundModule(), noFence)
	runID := httpEnrollment(t, env)
	s := env.DB.Scoped(fixtures.AcmeID)
	_, err := m.ExecuteHTTP(t.Context(), s, runID, 0, executionAttempt(), env.Cipher, httpResponse{status: http.StatusOK})
	require.NoError(t, err)

	decision, err := m.ExecuteHTTP(t.Context(), s, runID, 0, executionAttempt(), env.Cipher, httpResponse{status: http.StatusNotFound})

	require.NoError(t, err)
	assert.Equal(t, stepping.HTTPExecution{}, decision)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, 1, run.CurrentStep)
	queued := env.JobsOf(t, stepping.RunStepArgs{}.Kind())
	assert.Equal(t, []map[string]any{{"run_id": float64(runID), "step": float64(1)}}, queued)
}

func TestHTTPExecutionIgnoresErasedWork(t *testing.T) {
	env := testhelper.Setup(t)
	m := stepping.New(env.Bus, env.OutboundModule(), noFence)
	runID := httpEnrollment(t, env)
	s := env.DB.Scoped(fixtures.AcmeID)
	err := s.Contact().DeleteOneID(fixtures.ContactAliceID).Exec(t.Context())
	require.NoError(t, err)

	decision, err := m.ExecuteHTTP(t.Context(), s, runID, 0, executionAttempt(), env.Cipher, httpResponse{status: http.StatusNotFound})

	require.NoError(t, err)
	assert.Equal(t, stepping.HTTPExecution{}, decision)
	_, err = s.AutomationRun().Get(t.Context(), runID)
	assert.True(t, ent.IsNotFound(err))
	queued := env.JobsOf(t, stepping.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestHTTPExecutionCancelsADamagedStoredDefinition(t *testing.T) {
	env := testhelper.Setup(t)
	m := stepping.New(env.Bus, env.OutboundModule(), noFence)
	runID := httpEnrollment(t, env)
	s := env.DB.Scoped(fixtures.AcmeID)
	err := s.AutomationVersion().UpdateOneID(fixtures.AutomationNotifySlackVersionID).SetSteps("[").Exec(t.Context())
	require.NoError(t, err)

	decision, err := m.ExecuteHTTP(t.Context(), s, runID, 0, executionAttempt(), env.Cipher, httpResponse{status: http.StatusOK})

	require.NoError(t, err)
	require.Error(t, decision.Cancel)
	require.NoError(t, decision.Retry)
	assert.Zero(t, decision.Wait)
	run, err := s.AutomationRun().Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusFailed, run.Status)
	require.NotNil(t, run.FailureKind)
	assert.Equal(t, automationrun.FailureKindInvalidBody, *run.FailureKind)
	queued := env.JobsOf(t, stepping.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}
