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
	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/webhook"
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
	m := rules.New(env.Cipher, env.Bus)
	f := env.DB.RuleFiring.GetX(t.Context(), fixtures.RuleFiringHTTPPendingID)

	decision, err := m.ExecuteHTTP(context.Background(), env.DB.Scoped(fixtures.AcmeID), f.ID, 0, executionAttempt(), httpResponse{status: http.StatusOK}, executionNow)

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
	m := rules.New(env.Cipher, env.Bus)
	f := env.DB.RuleFiring.GetX(t.Context(), fixtures.RuleFiringHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)

	decision, err := m.ExecuteHTTP(context.Background(), s, f.ID, 0, executionAttempt(), httpResponse{status: http.StatusServiceUnavailable}, executionNow)

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
	decision, err = m.ExecuteHTTP(context.Background(), s, f.ID, 0, attempt, httpResponse{status: http.StatusOK}, executionNow)
	require.NoError(t, err)
	assert.Equal(t, rules.HTTPExecution{}, decision)
	stored = env.DB.RuleFiring.GetX(context.Background(), f.ID)
	results, err = rules.DecodeResults(stored.Results)
	require.NoError(t, err)
	assert.Equal(t, []rules.Result{{Type: rules.ActionHTTP, Status: rules.Succeeded}}, results)
	assert.NotNil(t, stored.CompletedAt)
}

func TestHTTPExecutionRecordsPermanentAndExhaustedFailures(t *testing.T) {
	cases := map[string]struct {
		status  int
		attempt int
		age     time.Duration
		detail  string
	}{
		"permanent refusal":  {status: http.StatusNotFound, attempt: 1, detail: "http_status 404: gone"},
		"exhausted retry":    {status: http.StatusInternalServerError, attempt: 3, detail: "http_status 500: gone"},
		"rate limit horizon": {status: http.StatusTooManyRequests, attempt: 1, age: webhook.HookRetryHorizon, detail: "http_status 429: gone"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			m := rules.New(env.Cipher, env.Bus)
			f := env.DB.RuleFiring.GetX(t.Context(), fixtures.RuleFiringHTTPPendingID)
			attempt := executionAttempt()
			attempt.Number = tc.attempt
			attempt.CreatedAt = attempt.CreatedAt.Add(-tc.age)

			decision, err := m.ExecuteHTTP(context.Background(), env.DB.Scoped(fixtures.AcmeID), f.ID, 0, attempt, httpResponse{status: tc.status, body: "gone"}, executionNow)

			require.NoError(t, err)
			assert.Equal(t, rules.HTTPExecution{}, decision)
			stored := env.DB.RuleFiring.GetX(context.Background(), f.ID)
			results, err := rules.DecodeResults(stored.Results)
			require.NoError(t, err)
			assert.Equal(t, []rules.Result{{Type: rules.ActionHTTP, Status: rules.Failed, Detail: tc.detail}}, results)
			assert.NotNil(t, stored.CompletedAt)
		})
	}
}

func TestHTTPExecutionRateLimitWaitsWithoutSpendingTheLastAttempt(t *testing.T) {
	env := testhelper.Setup(t)
	m := rules.New(env.Cipher, env.Bus)
	f := env.DB.RuleFiring.GetX(t.Context(), fixtures.RuleFiringHTTPPendingID)
	attempt := executionAttempt()
	attempt.Number = attempt.Limit

	decision, err := m.ExecuteHTTP(context.Background(), env.DB.Scoped(fixtures.AcmeID), f.ID, 0, attempt, httpResponse{status: http.StatusTooManyRequests, retryAfter: "120"}, executionNow)

	require.NoError(t, err)
	assert.NoError(t, decision.Retry)
	assert.Equal(t, 120*time.Second, decision.Wait)
	stored := env.DB.RuleFiring.GetX(context.Background(), f.ID)
	results, err := rules.DecodeResults(stored.Results)
	require.NoError(t, err)
	assert.Equal(t, []rules.Result{{Type: rules.ActionHTTP, Status: rules.Pending}}, results)
	assert.Nil(t, stored.CompletedAt)
}

func TestHTTPExecutionRejectsAnInvalidRenderedBody(t *testing.T) {
	env := testhelper.Setup(t)
	m := rules.New(env.Cipher, env.Bus)
	f := env.DB.RuleFiring.GetX(t.Context(), fixtures.RuleFiringHTTPPendingID)
	version := env.DB.RuleVersion.GetX(t.Context(), fixtures.RuleHTTPExecutionVersionID)
	actions, err := rules.DecodeActions(version.Actions)
	require.NoError(t, err)
	actions[0].Body = `{"n":{{ event.properties.n | default: 0 }}}`
	storedActions, err := json.Marshal(actions)
	require.NoError(t, err)
	err = env.DB.RuleVersion.UpdateOneID(version.ID).SetActions(string(storedActions)).Exec(t.Context())
	require.NoError(t, err)

	decision, err := m.ExecuteHTTP(context.Background(), env.DB.Scoped(fixtures.AcmeID), f.ID, 0, executionAttempt(), httpResponse{status: http.StatusOK}, executionNow)

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
	m := rules.New(env.Cipher, env.Bus)
	f := env.DB.RuleFiring.GetX(t.Context(), fixtures.RuleFiringHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)
	_, err := m.ExecuteHTTP(context.Background(), s, f.ID, 0, executionAttempt(), httpResponse{status: http.StatusOK}, executionNow)
	require.NoError(t, err)

	decision, err := m.ExecuteHTTP(context.Background(), s, f.ID, 0, executionAttempt(), httpResponse{status: http.StatusServiceUnavailable}, executionNow)

	require.NoError(t, err)
	assert.Equal(t, rules.HTTPExecution{}, decision)
	stored := env.DB.RuleFiring.GetX(context.Background(), f.ID)
	results, err := rules.DecodeResults(stored.Results)
	require.NoError(t, err)
	assert.Equal(t, []rules.Result{{Type: rules.ActionHTTP, Status: rules.Succeeded}}, results)
}

func TestHTTPExecutionOfAnErasedFiringDoesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	m := rules.New(env.Cipher, env.Bus)
	f := env.DB.RuleFiring.GetX(t.Context(), fixtures.RuleFiringHTTPPendingID)
	s := env.DB.Scoped(fixtures.AcmeID)
	err := rules.EraseFirings(context.Background(), s, fixtures.ContactBobID)
	require.NoError(t, err)

	decision, err := m.ExecuteHTTP(context.Background(), s, f.ID, 0, executionAttempt(), httpResponse{status: http.StatusServiceUnavailable}, executionNow)

	require.NoError(t, err)
	assert.Equal(t, rules.HTTPExecution{}, decision)
	exists, err := s.RuleFiring().Query().Where(rulefiring.ID(f.ID)).Exist(context.Background())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestHTTPExecutionRecordsAMissingContactAsFailed(t *testing.T) {
	env := testhelper.Setup(t)
	m := rules.New(env.Cipher, env.Bus)
	f := env.DB.RuleFiring.GetX(t.Context(), fixtures.RuleFiringHTTPPendingID)
	env.DB.Contact.Intercept(ent.InterceptFunc(func(ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(context.Context, ent.Query) (ent.Value, error) {
			return nil, &ent.NotFoundError{}
		})
	}))

	decision, err := m.ExecuteHTTP(context.Background(), env.DB.Scoped(fixtures.AcmeID), f.ID, 0, executionAttempt(), httpResponse{status: http.StatusOK}, executionNow)

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
	m := rules.New(env.Cipher, env.Bus)
	f := env.DB.RuleFiring.GetX(t.Context(), fixtures.RuleFiringTwoHTTPPendingID)

	decision, err := m.ExecuteHTTP(context.Background(), env.DB.Scoped(fixtures.AcmeID), f.ID, 1, executionAttempt(), httpResponse{status: http.StatusOK}, executionNow)

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
	m := rules.New(env.Cipher, env.Bus)
	s := env.DB.Scoped(fixtures.AcmeID)
	now := executionNow()
	response := httpResponse{
		status:     http.StatusTooManyRequests,
		retryAfter: "Sun, 18 Oct 2026 09:02:00 GMT",
		returned:   func() { now = now.Add(30 * time.Second) },
	}

	decision, err := m.ExecuteHTTP(t.Context(), s, fixtures.RuleFiringHTTPPendingID, 0, executionAttempt(), response, func() time.Time { return now })

	require.NoError(t, err)
	assert.Equal(t, rules.HTTPExecution{Wait: 90 * time.Second}, decision)
	stored, err := s.RuleFiring().Get(t.Context(), fixtures.RuleFiringHTTPPendingID)
	require.NoError(t, err)
	results, err := rules.DecodeResults(stored.Results)
	require.NoError(t, err)
	assert.Equal(t, []rules.Result{{Type: rules.ActionHTTP, Status: rules.Pending}}, results)
	assert.Nil(t, stored.CompletedAt)
}
