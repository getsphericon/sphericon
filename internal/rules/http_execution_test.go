package rules_test

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
	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/webhook"
)

type pendingHTTP struct{}

func (pendingHTTP) Run(context.Context, *ent.Scoped, rules.ActionInput) (rules.Result, error) {
	return rules.Result{Status: rules.Pending}, nil
}

type httpResponse struct {
	status     int
	body       string
	retryAfter string
}

func (r httpResponse) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: r.status, Body: io.NopCloser(strings.NewReader(r.body)), Header: http.Header{"Retry-After": []string{r.retryAfter}}}, nil
}

func pendingHTTPFiring(t *testing.T, env *testhelper.TestEnv, m *rules.Module, body string, props map[string]any) *ent.RuleFiring {
	t.Helper()
	return pendingHTTPActionsFiring(t, env, m, []rules.Action{{Type: rules.ActionHTTP, URL: hookURL, Body: body}}, props)
}

func pendingHTTPActionsFiring(t *testing.T, env *testhelper.TestEnv, m *rules.Module, actions []rules.Action, props map[string]any) *ent.RuleFiring {
	t.Helper()
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	r, err := m.Create(ctx, s, rules.CreateInput{
		Name: "Execute an HTTP Action", TriggerEvents: []string{"execute_http"},
		Actions: actions,
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, r.ID)
	require.NoError(t, err)
	fired, err := m.Evaluate(ctx, s, rules.Event{ContactID: fixtures.ContactBobID, Action: "execute_http", SourceID: "execute-http", RecordedAt: time.Now(), Properties: props})
	require.NoError(t, err)
	require.Len(t, fired, 1)
	err = m.RunActions(ctx, s, fired[0].ID, rules.ActionRunners{rules.ActionHTTP: pendingHTTP{}})
	require.NoError(t, err)
	return fired[0]
}

func executionAttempt() httpcall.Attempt {
	now := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	return httpcall.Attempt{Number: 1, Limit: 3, CreatedAt: now, Now: now}
}

func TestHTTPExecutionRecordsSuccessOnItsFiring(t *testing.T) {
	env := testhelper.Setup(t)
	m := rules.New(env.Cipher, env.Bus)
	f := pendingHTTPFiring(t, env, m, `{}`, nil)

	decision, err := m.ExecuteHTTP(context.Background(), env.DB.Scoped(fixtures.AcmeID), f.ID, 0, executionAttempt(), httpResponse{status: http.StatusOK})

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
	f := pendingHTTPFiring(t, env, m, `{}`, nil)
	s := env.DB.Scoped(fixtures.AcmeID)

	decision, err := m.ExecuteHTTP(context.Background(), s, f.ID, 0, executionAttempt(), httpResponse{status: http.StatusServiceUnavailable})

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
	decision, err = m.ExecuteHTTP(context.Background(), s, f.ID, 0, attempt, httpResponse{status: http.StatusOK})
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
			f := pendingHTTPFiring(t, env, m, `{}`, nil)
			attempt := executionAttempt()
			attempt.Number = tc.attempt
			attempt.Now = attempt.Now.Add(tc.age)

			decision, err := m.ExecuteHTTP(context.Background(), env.DB.Scoped(fixtures.AcmeID), f.ID, 0, attempt, httpResponse{status: tc.status, body: "gone"})

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
	f := pendingHTTPFiring(t, env, m, `{}`, nil)
	attempt := executionAttempt()
	attempt.Number = attempt.Limit

	decision, err := m.ExecuteHTTP(context.Background(), env.DB.Scoped(fixtures.AcmeID), f.ID, 0, attempt, httpResponse{status: http.StatusTooManyRequests, retryAfter: "120"})

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
	f := pendingHTTPFiring(t, env, m, `{"n":{{ event.properties.n | default: 0 }}}`, map[string]any{"n": "not a number"})

	decision, err := m.ExecuteHTTP(context.Background(), env.DB.Scoped(fixtures.AcmeID), f.ID, 0, executionAttempt(), httpResponse{status: http.StatusOK})

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
	f := pendingHTTPFiring(t, env, m, `{}`, nil)
	s := env.DB.Scoped(fixtures.AcmeID)
	_, err := m.ExecuteHTTP(context.Background(), s, f.ID, 0, executionAttempt(), httpResponse{status: http.StatusOK})
	require.NoError(t, err)

	decision, err := m.ExecuteHTTP(context.Background(), s, f.ID, 0, executionAttempt(), httpResponse{status: http.StatusServiceUnavailable})

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
	f := pendingHTTPFiring(t, env, m, `{}`, nil)
	s := env.DB.Scoped(fixtures.AcmeID)
	err := rules.EraseFirings(context.Background(), s, fixtures.ContactBobID)
	require.NoError(t, err)

	decision, err := m.ExecuteHTTP(context.Background(), s, f.ID, 0, executionAttempt(), httpResponse{status: http.StatusServiceUnavailable})

	require.NoError(t, err)
	assert.Equal(t, rules.HTTPExecution{}, decision)
	exists, err := s.RuleFiring().Query().Where(rulefiring.ID(f.ID)).Exist(context.Background())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestHTTPExecutionRecordsAMissingContactAsFailed(t *testing.T) {
	env := testhelper.Setup(t)
	m := rules.New(env.Cipher, env.Bus)
	f := pendingHTTPFiring(t, env, m, `{}`, nil)
	env.DB.Contact.Intercept(ent.InterceptFunc(func(ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(context.Context, ent.Query) (ent.Value, error) {
			return nil, &ent.NotFoundError{}
		})
	}))

	decision, err := m.ExecuteHTTP(context.Background(), env.DB.Scoped(fixtures.AcmeID), f.ID, 0, executionAttempt(), httpResponse{status: http.StatusOK})

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
	f := pendingHTTPActionsFiring(t, env, m, []rules.Action{
		{Type: rules.ActionHTTP, URL: hookURL, Body: `{}`},
		{Type: rules.ActionHTTP, URL: hookURL, Body: `{}`},
	}, nil)

	decision, err := m.ExecuteHTTP(context.Background(), env.DB.Scoped(fixtures.AcmeID), f.ID, 1, executionAttempt(), httpResponse{status: http.StatusOK})

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
