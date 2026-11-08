package rules_test

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
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
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

func executionNow() time.Time {
	return time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
}

func executionAttempt() httpcall.Attempt {
	now := executionNow()
	return httpcall.Attempt{Number: 1, Limit: 3, CreatedAt: now}
}

func TestHTTPExecutionRecordsSuccessOnItsFiring(t *testing.T) {
	env := testhelper.Setup(t)
	f := env.DB.RuleFiring.GetX(t.Context(), fixtures.RuleFiringHTTPPendingID)

	decision, err := rules.New(env.Cipher, env.Bus, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(context.Background(), env.DB.Scoped(fixtures.AcmeID), f.ID, 0, executionAttempt())

	require.NoError(t, err)
	assert.Equal(t, rules.HTTPExecution{}, decision)
	stored := env.DB.RuleFiring.GetX(context.Background(), f.ID)
	results, err := rules.DecodeResults(stored.Results)
	require.NoError(t, err)
	assert.Equal(t, []rules.Result{{Type: rules.ActionHTTP, Status: rules.Succeeded}}, results)
	assert.NotNil(t, stored.CompletedAt)
}

func TestHTTPExecutionLeavesTemporaryFailuresPendingUntilRecovery(t *testing.T) {
	env := testhelper.Setup(t)
	f := env.DB.RuleFiring.GetX(t.Context(), fixtures.RuleFiringHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)

	decision, err := rules.New(env.Cipher, env.Bus, httpResponse{status: http.StatusServiceUnavailable}, executionNow).ExecuteHTTP(context.Background(), s, f.ID, 0, executionAttempt())

	require.NoError(t, err)
	require.Error(t, decision.Retry)
	assert.Zero(t, decision.Wait)
	stored := env.DB.RuleFiring.GetX(context.Background(), f.ID)
	results, err := rules.DecodeResults(stored.Results)
	require.NoError(t, err)
	assert.Equal(t, []rules.Result{{Type: rules.ActionHTTP, Status: rules.Pending}}, results)
	assert.Nil(t, stored.CompletedAt)

	attempt := executionAttempt()
	attempt.Number = 2
	decision, err = rules.New(env.Cipher, env.Bus, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(context.Background(), s, f.ID, 0, attempt)
	require.NoError(t, err)
	assert.Equal(t, rules.HTTPExecution{}, decision)
	stored = env.DB.RuleFiring.GetX(context.Background(), f.ID)
	results, err = rules.DecodeResults(stored.Results)
	require.NoError(t, err)
	assert.Equal(t, []rules.Result{{Type: rules.ActionHTTP, Status: rules.Succeeded}}, results)
	assert.NotNil(t, stored.CompletedAt)
}

func TestHTTPExecutionRecordsAGivenUpCallAsFailed(t *testing.T) {
	env := testhelper.Setup(t)
	f := env.DB.RuleFiring.GetX(t.Context(), fixtures.RuleFiringHTTPPendingID)

	decision, err := rules.New(env.Cipher, env.Bus, httpResponse{status: http.StatusNotFound, body: "gone"}, executionNow).ExecuteHTTP(context.Background(), env.DB.Scoped(fixtures.AcmeID), f.ID, 0, executionAttempt())

	require.NoError(t, err)
	assert.Equal(t, rules.HTTPExecution{}, decision)
	stored := env.DB.RuleFiring.GetX(context.Background(), f.ID)
	results, err := rules.DecodeResults(stored.Results)
	require.NoError(t, err)
	assert.Equal(t, []rules.Result{{Type: rules.ActionHTTP, Status: rules.Failed, Detail: "http_status 404: gone"}}, results)
	assert.NotNil(t, stored.CompletedAt)
}

func TestHTTPExecutionRejectsAnInvalidRenderedBody(t *testing.T) {
	env := testhelper.Setup(t)
	f := env.DB.RuleFiring.GetX(t.Context(), fixtures.RuleFiringHTTPPendingID)
	version := env.DB.RuleVersion.GetX(t.Context(), fixtures.RuleHTTPExecutionVersionID)
	actions, err := rules.DecodeActions(version.Actions)
	require.NoError(t, err)
	actions[0].Body = `{"n":{{ event.properties.n | default: 0 }}}`
	storedActions, err := json.Marshal(actions)
	require.NoError(t, err)
	err = env.DB.RuleVersion.UpdateOneID(version.ID).SetActions(string(storedActions)).Exec(t.Context())
	require.NoError(t, err)

	decision, err := rules.New(env.Cipher, env.Bus, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(context.Background(), env.DB.Scoped(fixtures.AcmeID), f.ID, 0, executionAttempt())

	require.NoError(t, err)
	assert.Equal(t, rules.HTTPExecution{}, decision)
	stored := env.DB.RuleFiring.GetX(context.Background(), f.ID)
	results, err := rules.DecodeResults(stored.Results)
	require.NoError(t, err)
	assert.Equal(t, rules.Failed, results[0].Status)
	assert.Contains(t, results[0].Detail, "invalid_body: ")
	assert.NotNil(t, stored.CompletedAt)
}

func TestHTTPExecutionDoesNotRepeatADecidedAction(t *testing.T) {
	env := testhelper.Setup(t)
	f := env.DB.RuleFiring.GetX(t.Context(), fixtures.RuleFiringHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)
	_, err := rules.New(env.Cipher, env.Bus, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(context.Background(), s, f.ID, 0, executionAttempt())
	require.NoError(t, err)

	decision, err := rules.New(env.Cipher, env.Bus, httpResponse{status: http.StatusServiceUnavailable}, executionNow).ExecuteHTTP(context.Background(), s, f.ID, 0, executionAttempt())

	require.NoError(t, err)
	assert.Equal(t, rules.HTTPExecution{}, decision)
	stored := env.DB.RuleFiring.GetX(context.Background(), f.ID)
	results, err := rules.DecodeResults(stored.Results)
	require.NoError(t, err)
	assert.Equal(t, []rules.Result{{Type: rules.ActionHTTP, Status: rules.Succeeded}}, results)
}

func TestHTTPExecutionOfAnErasedFiringDoesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	f := env.DB.RuleFiring.GetX(t.Context(), fixtures.RuleFiringHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)
	// The Contact's Erasure deletes its Firings by their cascading foreign key.
	env.DB.RuleFiring.DeleteOneID(f.ID).ExecX(t.Context())

	decision, err := rules.New(env.Cipher, env.Bus, httpResponse{status: http.StatusServiceUnavailable}, executionNow).ExecuteHTTP(context.Background(), s, f.ID, 0, executionAttempt())

	require.NoError(t, err)
	assert.Equal(t, rules.HTTPExecution{}, decision)
}

func TestHTTPExecutionRecordsAMissingContactAsFailed(t *testing.T) {
	env := testhelper.Setup(t)
	f := env.DB.RuleFiring.GetX(t.Context(), fixtures.RuleFiringHTTPPendingID)
	env.DB.Contact.Intercept(ent.InterceptFunc(func(ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(context.Context, ent.Query) (ent.Value, error) {
			return nil, &ent.NotFoundError{}
		})
	}))

	decision, err := rules.New(env.Cipher, env.Bus, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(context.Background(), env.DB.Scoped(fixtures.AcmeID), f.ID, 0, executionAttempt())

	require.NoError(t, err)
	assert.Equal(t, rules.HTTPExecution{}, decision)
	stored := env.DB.RuleFiring.GetX(context.Background(), f.ID)
	results, err := rules.DecodeResults(stored.Results)
	require.NoError(t, err)
	assert.Equal(t, []rules.Result{{Type: rules.ActionHTTP, Status: rules.Failed, Detail: "contact not found"}}, results)
	assert.NotNil(t, stored.CompletedAt)
}

func TestHTTPExecutionDecidesOnlyTheSelectedPendingAction(t *testing.T) {
	env := testhelper.Setup(t)
	f := env.DB.RuleFiring.GetX(t.Context(), fixtures.RuleFiringTwoHTTPPendingID)

	decision, err := rules.New(env.Cipher, env.Bus, httpResponse{status: http.StatusOK}, executionNow).ExecuteHTTP(context.Background(), env.DB.Scoped(fixtures.AcmeID), f.ID, 1, executionAttempt())

	require.NoError(t, err)
	assert.Equal(t, rules.HTTPExecution{}, decision)
	stored := env.DB.RuleFiring.GetX(context.Background(), f.ID)
	results, err := rules.DecodeResults(stored.Results)
	require.NoError(t, err)
	assert.Equal(t, []rules.Result{
		{Type: rules.ActionHTTP, Status: rules.Pending},
		{Type: rules.ActionHTTP, Status: rules.Succeeded},
	}, results)
	assert.Nil(t, stored.CompletedAt)
}

func TestHTTPExecutionWaitsUntilRetryAfterFromTheResponseTime(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	now := executionNow()
	response := httpResponse{
		status:     http.StatusTooManyRequests,
		retryAfter: "Sun, 18 Oct 2026 09:02:00 GMT",
		returned:   func() { now = now.Add(30 * time.Second) },
	}

	decision, err := rules.New(env.Cipher, env.Bus, response, func() time.Time { return now }).ExecuteHTTP(t.Context(), s, fixtures.RuleFiringHTTPPendingID, 0, executionAttempt())

	require.NoError(t, err)
	assert.Equal(t, rules.HTTPExecution{Wait: 90 * time.Second}, decision)
	stored, err := s.RuleFiring().Get(t.Context(), fixtures.RuleFiringHTTPPendingID)
	require.NoError(t, err)
	results, err := rules.DecodeResults(stored.Results)
	require.NoError(t, err)
	assert.Equal(t, []rules.Result{{Type: rules.ActionHTTP, Status: rules.Pending}}, results)
	assert.Nil(t, stored.CompletedAt)
}
