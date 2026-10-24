package jobs_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/webhook"
)

// recoverAfterFirstCall makes the receiver answer 503 to the first call and 200 to every one after.
func recoverAfterFirstCall(rcv *hookReceiver) {
	rcv.respond(http.StatusServiceUnavailable, "try later", "")
	rcv.onCall = func() {
		if len(rcv.received()) == 1 {
			rcv.respond(http.StatusOK, "", "")
		}
	}
}

const testSigner = "dGVzdC1zZWNyZXQ="

func ruleRunners(env *testhelper.TestEnv) rules.ActionRunners {
	return jobs.RuleActionRunners(newMod(env, nil), env.Bus)
}

// httpRuleHarness fires a Rule that calls a receiver and then applies a tag, runs the Actions job,
// and works the call's job the way river would.
type httpRuleHarness struct {
	env    *testhelper.TestEnv
	rcv    *hookReceiver
	firing *ent.RuleFiring
	clock  time.Time
}

// fireHTTPRule creates and activates the Rule, fires it for Bob with the Event's properties and
// runs its Actions job, which leaves the call Pending on a job of its own.
func fireHTTPRule(t *testing.T, env *testhelper.TestEnv, body string, props map[string]any) *httpRuleHarness {
	t.Helper()
	ctx := context.Background()
	rcv := newHookReceiver(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := rules.New(env.Cipher, env.Bus)
	r, err := m.Create(ctx, s, rules.CreateInput{
		Name:          "Call the hook",
		TriggerEvents: []string{"rule_http"},
		Actions: []rules.Action{
			{
				Type: rules.ActionHTTP, URL: rcv.srv.URL, Body: body, Secret: testSigner,
				Headers: []httpcall.Header{{Name: "Authorization", Value: "Bearer hunter2"}},
			},
			{Type: rules.ActionApplyTag, Tag: "after-hook"},
		},
	})
	require.NoError(t, err)
	_, err = m.Activate(ctx, s, r.ID)
	require.NoError(t, err)
	fired, err := m.Evaluate(ctx, s, rules.Event{
		ContactID: fixtures.ContactBobID, Action: "rule_http", SourceID: "evt-http",
		RecordedAt: time.Now(), Properties: props,
	})
	require.NoError(t, err)
	require.Len(t, fired, 1)
	require.NoError(t, m.RunActions(ctx, s, fired[0].ID, ruleRunners(env)))
	return &httpRuleHarness{env: env, rcv: rcv, firing: fired[0], clock: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
}

func (h *httpRuleHarness) reload() *ent.RuleFiring {
	return h.env.DB.RuleFiring.GetX(context.Background(), h.firing.ID)
}

func (h *httpRuleHarness) worker(client webhook.Doer) *jobs.RunRuleHTTPWorker {
	return jobs.NewRunRuleHTTPWorker(h.env.DB, rules.New(h.env.Cipher, h.env.Bus), client, func() time.Time { return h.clock })
}

func (h *httpRuleHarness) allowing() *jobs.RunRuleHTTPWorker {
	return h.worker(webhook.NewClientAllowing(2*time.Second, h.rcv.allow))
}

// attempt works the call's job as river's attempt-th try; the last one is the job's MaxAttempts.
func (h *httpRuleHarness) attempt(w *jobs.RunRuleHTTPWorker, attempt int) error {
	return w.Work(context.Background(), &river.Job[jobs.RunRuleHTTPArgs]{
		JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: jobs.RunRuleHTTPArgs{}.InsertOpts().MaxAttempts, CreatedAt: h.clock},
		Args:   jobs.RunRuleHTTPArgs{FiringID: h.firing.ID},
	})
}

func (h *httpRuleHarness) lastAttempt() int { return jobs.RunRuleHTTPArgs{}.InsertOpts().MaxAttempts }

func TestAnHTTPActionIsPendingWhileTheActionsAfterItRun(t *testing.T) {
	env := testhelper.Setup(t)

	h := fireHTTPRule(t, env, `{}`, nil)

	f := h.reload()
	results := resultsOf(t, f)
	require.Len(t, results, 2)
	assert.Equal(t, rules.Result{Type: rules.ActionHTTP, Status: rules.Pending}, results[0])
	assert.Equal(t, rules.Succeeded, results[1].Status, "the next Action does not wait for the call")
	assert.Nil(t, f.CompletedAt, "the Firing finishes when the call is recorded")
	assert.Equal(t, []string{"after-hook"}, tagsOf(t, env, fixtures.ContactBobID))
	assert.Empty(t, h.rcv.received(), "the Actions job makes no call")
	assert.Equal(t, []map[string]any{{"firing_id": float64(h.firing.ID), "index": float64(0)}}, env.JobsOf(t, jobs.RunRuleHTTPArgs{}.Kind()))
}

func TestAnHTTPActionPostsTheContactAndTheEventAndFinishesTheFiring(t *testing.T) {
	env := testhelper.Setup(t)
	body := `{"email":"{{ email }}","event":"{{ event.action }}","plan":"{{ event.properties.plan }}","missing":"[{{ event.properties.nope }}]"}`
	h := fireHTTPRule(t, env, body, map[string]any{"plan": `pro "plus"`})

	require.NoError(t, h.attempt(h.allowing(), 1))

	calls := h.rcv.received()
	require.Len(t, calls, 1)
	assert.JSONEq(t, `{"email":"bob@example.com","event":"rule_http","plan":"pro \"plus\"","missing":"[]"}`, calls[0].body)
	assert.Equal(t, "Bearer hunter2", calls[0].header.Get("Authorization"))
	assert.NotEmpty(t, calls[0].header.Get(standardwebhooks.HeaderWebhookSignature), "a secret signs the call")
	f := h.reload()
	results := resultsOf(t, f)
	require.Len(t, results, 2)
	assert.Equal(t, rules.Result{Type: rules.ActionHTTP, Status: rules.Succeeded}, results[0])
	assert.Equal(t, rules.Succeeded, results[1].Status)
	assert.NotNil(t, f.CompletedAt)
}

func TestAnHTTPActionRetriesAServerErrorWithoutRecordingIt(t *testing.T) {
	env := testhelper.Setup(t)
	h := fireHTTPRule(t, env, `{}`, nil)
	recoverAfterFirstCall(h.rcv)
	w := h.allowing()

	require.Error(t, h.attempt(w, 1), "river retries the job")
	assert.Equal(t, rules.Pending, resultsOf(t, h.reload())[0].Status)

	require.NoError(t, h.attempt(w, 2))
	assert.Len(t, h.rcv.received(), 2)
	f := h.reload()
	assert.Equal(t, rules.Succeeded, resultsOf(t, f)[0].Status)
	assert.NotNil(t, f.CompletedAt)
}

func TestAnHTTPActionThatKeepsFailingIsRecordedFailedWithTheStatusAndTheAnswerOnItsLastAttempt(t *testing.T) {
	env := testhelper.Setup(t)
	h := fireHTTPRule(t, env, `{}`, nil)
	h.rcv.respond(http.StatusInternalServerError, "boom", "")
	w := h.allowing()

	require.Error(t, h.attempt(w, h.lastAttempt()-1))
	require.NoError(t, h.attempt(w, h.lastAttempt()))

	f := h.reload()
	results := resultsOf(t, f)
	assert.Equal(t, rules.Failed, results[0].Status)
	assert.Equal(t, "http_status 500: boom", results[0].Detail)
	assert.Equal(t, rules.Succeeded, results[1].Status, "a failed Action does not cancel the others")
	assert.NotNil(t, f.CompletedAt)
}

func TestAnHTTPActionIsNotRetriedOnAClientError(t *testing.T) {
	env := testhelper.Setup(t)
	h := fireHTTPRule(t, env, `{}`, nil)
	h.rcv.respond(http.StatusNotFound, "gone", "")

	require.NoError(t, h.attempt(h.allowing(), 1), "a 404 would answer the same: the job ends")

	assert.Len(t, h.rcv.received(), 1)
	results := resultsOf(t, h.reload())
	assert.Equal(t, rules.Failed, results[0].Status)
	assert.Equal(t, "http_status 404: gone", results[0].Detail)
}

func TestAnHTTPActionKeepsOnlyTheFirstKiBOfTheAnswer(t *testing.T) {
	env := testhelper.Setup(t)
	h := fireHTTPRule(t, env, `{}`, nil)
	h.rcv.respond(http.StatusBadRequest, strings.Repeat("x", 5000), "")

	require.NoError(t, h.attempt(h.allowing(), h.lastAttempt()))

	assert.Len(t, resultsOf(t, h.reload())[0].Detail, len("http_status 400: ")+webhook.ExcerptBytes)
}

func TestAnHTTPActionDoesNotCallABlockedAddress(t *testing.T) {
	env := testhelper.Setup(t)
	h := fireHTTPRule(t, env, `{}`, nil)

	// The Instance allowlist does not name the receiver's loopback address, and no retry opens it.
	require.NoError(t, h.attempt(h.worker(webhook.NewClient(2*time.Second)), 1))

	assert.Empty(t, h.rcv.received())
	results := resultsOf(t, h.reload())
	assert.Equal(t, rules.Failed, results[0].Status)
	assert.Equal(t, string(webhook.KindBlockedAddress), results[0].Detail)
	assert.NotContains(t, results[0].Detail, h.rcv.srv.URL, "the URL is a secret")
}

func TestAnHTTPActionWithABodyThatDoesNotRenderFailsWithoutCalling(t *testing.T) {
	env := testhelper.Setup(t)
	// Valid as saved (no Contact, no Event), invalid once the property is not a number.
	body := `{"n":{{ event.properties.n | default: 0 }}}`
	h := fireHTTPRule(t, env, body, map[string]any{"n": "not a number"})

	require.NoError(t, h.attempt(h.allowing(), 1))

	assert.Empty(t, h.rcv.received())
	results := resultsOf(t, h.reload())
	assert.Equal(t, rules.Failed, results[0].Status)
	assert.Contains(t, results[0].Detail, "invalid_body")
}

func TestAnHTTPActionRateLimitSnoozesWithoutSpendingAnAttempt(t *testing.T) {
	env := testhelper.Setup(t)
	h := fireHTTPRule(t, env, `{}`, nil)
	h.rcv.respond(http.StatusTooManyRequests, "slow down", "120")

	// Even the last attempt is not failed by a 429: it snoozes.
	err := h.attempt(h.allowing(), h.lastAttempt())

	var snooze *rivertype.JobSnoozeError
	require.ErrorAs(t, err, &snooze)
	assert.Equal(t, 120*time.Second, snooze.Duration)
	assert.Equal(t, rules.Pending, resultsOf(t, h.reload())[0].Status)
}

func TestAnHTTPActionRateLimitEndsAtTheRetryHorizon(t *testing.T) {
	env := testhelper.Setup(t)
	h := fireHTTPRule(t, env, `{}`, nil)
	h.rcv.respond(http.StatusTooManyRequests, "", "")
	w := h.allowing()
	h.clock = h.clock.Add(webhook.HookRetryHorizon) // the job was created a horizon ago

	err := w.Work(context.Background(), &river.Job[jobs.RunRuleHTTPArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3, CreatedAt: h.clock.Add(-webhook.HookRetryHorizon)},
		Args:   jobs.RunRuleHTTPArgs{FiringID: h.firing.ID},
	})

	require.NoError(t, err)
	results := resultsOf(t, h.reload())
	assert.Equal(t, rules.Failed, results[0].Status)
	assert.Equal(t, "http_status 429", results[0].Detail)
}

func TestAnHTTPActionAlreadyRecordedIsNotCalledAgain(t *testing.T) {
	env := testhelper.Setup(t)
	h := fireHTTPRule(t, env, `{}`, nil)
	w := h.allowing()
	require.NoError(t, h.attempt(w, 1))

	require.NoError(t, h.attempt(w, 2), "a redelivered job")

	assert.Len(t, h.rcv.received(), 1)
}

func TestAnHTTPActionOfAnErasedFiringCallsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	h := fireHTTPRule(t, env, `{}`, nil)
	env.DB.RuleFiring.DeleteOneID(h.firing.ID).ExecX(context.Background())

	require.NoError(t, h.attempt(h.allowing(), 1))

	assert.Empty(t, h.rcv.received())
}

func TestAnHTTPActionRunsOnAQueueOfItsOwnWithTheRetryHorizonOfAHook(t *testing.T) {
	env := testhelper.Setup(t)
	h := fireHTTPRule(t, env, `{}`, nil)

	// The Actions job queues the call with the options its args declare.
	opts := jobs.RunRuleHTTPArgs{}.InsertOpts()
	assert.Equal(t, stepping.QueueHTTPSteps, opts.Queue)
	assert.NotEqual(t, jobs.QueueWebhooks, opts.Queue)
	assert.Equal(t, webhook.HookMaxAttempts(), opts.MaxAttempts)
	assert.True(t, opts.UniqueOpts.ByArgs, "a redelivered Actions job does not queue the call twice")

	next := h.allowing().NextRetry(&river.Job[jobs.RunRuleHTTPArgs]{JobRow: &rivertype.JobRow{Attempt: 2, AttemptedAt: &h.clock}})
	assert.Equal(t, webhook.HookNextRetry(h.clock, 2), next)
}
