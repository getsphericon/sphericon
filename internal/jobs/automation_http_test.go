package jobs_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/stepping"
)

// hookReceiver is a third party's endpoint: it records every call and answers as told.
type hookReceiver struct {
	srv   *httptest.Server
	addr  *url.URL
	allow webhook.Allowlist

	mu         sync.Mutex
	status     int
	retryAfter string
	answer     string
	calls      []hookCall
	// onCall runs when a call arrives, before the receiver answers: a test blocks it to hold
	// the call in flight.
	onCall func()
}

type hookCall struct {
	header http.Header
	body   string
}

func newHookReceiver(t *testing.T) *hookReceiver {
	t.Helper()
	r := &hookReceiver{status: http.StatusOK, onCall: func() {}}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		r.onCall()
		r.mu.Lock()
		defer r.mu.Unlock()
		r.calls = append(r.calls, hookCall{header: req.Header.Clone(), body: string(body)})
		if r.retryAfter != "" {
			w.Header().Set("Retry-After", r.retryAfter)
		}
		w.WriteHeader(r.status)
		_, _ = fmt.Fprint(w, r.answer)
	}))
	t.Cleanup(r.srv.Close)
	u, err := url.Parse(r.srv.URL)
	require.NoError(t, err)
	r.addr = u
	r.allow, err = webhook.ParseAllowlist(u.Host)
	require.NoError(t, err)
	return r
}

// fixtureHookHost is the host the Quahog Rule and Automations call (fixtures/rule_versions.yml,
// fixtures/automation_versions.yml): http://hooks.quahog.test/notify.
const fixtureHookHost = "hooks.quahog.test"

// routed is client with the calls to the fixture hook host sent to the receiver: the request is
// the one the code made, only its address is the receiver's, and client's address checks judge
// that address. A .test host resolves nowhere, so a call it does not route fails.
func (r *hookReceiver) routed(client webhook.Doer) webhook.Doer {
	return routingDoer{to: r.addr, next: client}
}

type routingDoer struct {
	to   *url.URL
	next webhook.Doer
}

func (d routingDoer) Do(req *http.Request) (*http.Response, error) {
	if req.URL.Host == fixtureHookHost {
		req = req.Clone(req.Context())
		req.URL.Scheme, req.URL.Host, req.Host = d.to.Scheme, d.to.Host, ""
	}
	return d.next.Do(req)
}

func (r *hookReceiver) respond(status int, answer, retryAfter string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status, r.answer, r.retryAfter = status, answer, retryAfter
}

func (r *hookReceiver) received() []hookCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]hookCall(nil), r.calls...)
}

// httpStepHarness works the HTTP step of Quinn's Enrollment in one of the Quahog HTTP
// Automations, whose steps are an HTTP step to the receiver and a tag step after it.
type httpStepHarness struct {
	*riverEnv

	rcv   *hookReceiver
	clock time.Time
}

func newHTTPStepHarness(t *testing.T) *httpStepHarness {
	t.Helper()
	return &httpStepHarness{riverEnv: newRiverEnv(t), rcv: newHookReceiver(t), clock: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
}

// reachHTTPStep queues the step job of the Enrollment and runs it, which parks the Enrollment on
// the HTTP step's job of its own.
func (h *httpStepHarness) reachHTTPStep(t *testing.T, runID int64) {
	t.Helper()
	queueStep(t, h.TestEnv, runID)
	require.True(t, h.RunNextStep(t, h.stepper()))
	require.Equal(t, []map[string]any{{"run_id": float64(runID), "step": float64(0)}}, h.JobsOf(t, stepping.RunHTTPStepArgs{}.Kind()))
}

func (h *httpStepHarness) stepper() *stepping.Module {
	return stepperOf(h.TestEnv, resolvingTo(h.sender))
}

func (h *httpStepHarness) worker(allow webhook.Allowlist) *jobs.RunHTTPStepWorker {
	return jobs.NewRunHTTPStepWorker(h.DB, h.cipher, h.stepper(), h.rcv.routed(webhook.NewClientAllowing(2*time.Second, allow)), func() time.Time { return h.clock })
}

// attempt works the HTTP step as river's attempt-th try; the last one is the job's MaxAttempts.
func (h *httpStepHarness) attempt(w *jobs.RunHTTPStepWorker, runID int64, attempt int) error {
	return w.Work(h.workCtx(), &river.Job[stepping.RunHTTPStepArgs]{
		JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: stepping.RunHTTPStepArgs{}.InsertOpts().MaxAttempts, CreatedAt: h.clock},
		Args:   stepping.RunHTTPStepArgs{RunID: runID},
	})
}

func (h *httpStepHarness) run(runID int64) *automationrunView {
	r := h.DB.AutomationRun.GetX(context.Background(), runID)
	return &automationrunView{status: r.Status, step: r.CurrentStep, kind: r.FailureKind, httpStatus: r.FailureStatus, excerpt: r.FailureExcerpt}
}

type automationrunView struct {
	status     automationrun.Status
	step       int
	kind       *automationrun.FailureKind
	httpStatus *int
	excerpt    *string
}

func TestHTTPStepContinuesAfterA2xxSigned(t *testing.T) {
	h := newHTTPStepHarness(t)
	runID := int64(fixtures.AutomationRunQuahogHookSignedID)

	require.NoError(t, h.attempt(h.worker(h.rcv.allow), runID, 1))

	calls := h.rcv.received()
	require.Len(t, calls, 1)
	assert.JSONEq(t, `{"email":"`+fixtures.ContactQuahogQuinnEmail+`"}`, calls[0].body)
	assert.Equal(t, "Bearer slack-demo-token", calls[0].header.Get("Authorization"))
	wh, err := standardwebhooks.NewWebhook(webhookSecret)
	require.NoError(t, err)
	assert.NoError(t, wh.Verify([]byte(calls[0].body), calls[0].header), "a step with a secret is signed")

	r := h.run(runID)
	assert.Equal(t, automationrun.StatusActive, r.status)
	assert.Equal(t, 1, r.step, "the Enrollment moved past the HTTP step")
	assert.Equal(t, []map[string]any{stepJob(runID, 1)}, queuedSteps(t, h.TestEnv), "and its next step is queued")
}

// An HTTP step renders the Event that started the Enrollment as a Rule's http Action does (#752):
// `event.action` and `event.properties`, JSON-escaped, and a property the Event lacks empty.
func TestHTTPStepRendersTheTriggeringEventsProperties(t *testing.T) {
	// The Enrollment began on an http_demo Event; the body puts in its action, amount and plan and a
	// property it lacks.
	h := newHTTPStepHarness(t)
	runID := int64(fixtures.AutomationRunQuahogHookEventID)

	require.NoError(t, h.attempt(h.worker(h.rcv.allow), runID, 1))

	calls := h.rcv.received()
	require.Len(t, calls, 1)
	assert.JSONEq(t, `{"action":"http_demo","amount":"€49","plan":"<b>Pro</b> \"annual\"","missing":""}`, calls[0].body)
}

func TestHTTPStepWithoutASecretIsUnsigned(t *testing.T) {
	h := newHTTPStepHarness(t)
	runID := int64(fixtures.AutomationRunQuahogHookUnsignedID)

	require.NoError(t, h.attempt(h.worker(h.rcv.allow), runID, 1))

	calls := h.rcv.received()
	require.Len(t, calls, 1)
	assert.Empty(t, calls[0].header.Get(standardwebhooks.HeaderWebhookSignature))
	assert.Equal(t, 1, h.run(runID).step)
}

func TestHTTPStepDeliveredAgainAfterSuccessMakesNoNewCallOrNextJob(t *testing.T) {
	h := newHTTPStepHarness(t)
	runID := int64(fixtures.AutomationRunQuahogHookID)
	w := h.worker(h.rcv.allow)
	require.NoError(t, h.attempt(w, runID, 1))

	require.NoError(t, h.attempt(w, runID, 2))

	calls := h.rcv.received()
	assert.Len(t, calls, 1)
	r := h.run(runID)
	assert.Equal(t, 1, r.step)
	queued := queuedSteps(t, h.TestEnv)
	assert.Equal(t, []map[string]any{stepJob(runID, 1)}, queued)
}

func TestHTTPStepFailsTheEnrollmentWhenRetriesAreSpent(t *testing.T) {
	h := newHTTPStepHarness(t)
	runID := int64(fixtures.AutomationRunQuahogHookID)
	h.rcv.respond(http.StatusServiceUnavailable, strings.Repeat("x", 3000), "")
	w := h.worker(h.rcv.allow)

	require.Error(t, h.attempt(w, runID, 1))
	assert.Equal(t, automationrun.StatusActive, h.run(runID).status, "a retry is still to come")

	require.Error(t, h.attempt(w, runID, stepping.RunHTTPStepArgs{}.InsertOpts().MaxAttempts))
	r := h.run(runID)
	assert.Equal(t, automationrun.StatusFailed, r.status)
	assert.Equal(t, 0, r.step, "nothing is skipped")
	require.NotNil(t, r.kind)
	assert.Equal(t, automationrun.FailureKindHTTPStatus, *r.kind)
	assert.Equal(t, http.StatusServiceUnavailable, *r.httpStatus)
	assert.Equal(t, strings.Repeat("x", webhook.ExcerptBytes), *r.excerpt)
	assert.Empty(t, queuedSteps(t, h.TestEnv), "a failed Enrollment queues no next step")
	calls := h.rcv.received()
	require.Len(t, calls, 2)
	firstID := calls[0].header.Get(standardwebhooks.HeaderWebhookID)
	retryID := calls[1].header.Get(standardwebhooks.HeaderWebhookID)
	assert.Equal(t, fmt.Sprintf("automation_%d_0", runID), firstID)
	assert.Equal(t, firstID, retryID, "an ordinary retry retains the external request identity")
}

func TestHTTPStepAnsweredWithAClientErrorFailsAtOnce(t *testing.T) {
	h := newHTTPStepHarness(t)
	runID := int64(fixtures.AutomationRunQuahogHookID)
	h.rcv.respond(http.StatusNotFound, "gone", "")

	err := h.attempt(h.worker(h.rcv.allow), runID, 1)

	var cancelled *rivertype.JobCancelError
	require.ErrorAs(t, err, &cancelled)
	r := h.run(runID)
	assert.Equal(t, automationrun.StatusFailed, r.status)
	assert.Equal(t, http.StatusNotFound, *r.httpStatus)
}

func TestHTTPStepToABlockedAddressFailsAtOnce(t *testing.T) {
	h := newHTTPStepHarness(t)
	runID := int64(fixtures.AutomationRunQuahogHookID)

	err := h.attempt(h.worker(webhook.Allowlist{}), runID, 1)

	var cancelled *rivertype.JobCancelError
	require.ErrorAs(t, err, &cancelled, "no retry opens a blocked address")
	r := h.run(runID)
	assert.Equal(t, automationrun.StatusFailed, r.status)
	assert.Equal(t, automationrun.FailureKindBlockedAddress, *r.kind)
	assert.Nil(t, r.httpStatus)
	assert.Empty(t, h.rcv.received())
}

func TestHTTPStepWhoseBodyIsNotJSONFailsAtOnce(t *testing.T) {
	// Valid with no Contact at save time, not once a first name is put in unquoted.
	h := newHTTPStepHarness(t)
	runID := int64(fixtures.AutomationRunQuahogHookNumberID)

	err := h.attempt(h.worker(h.rcv.allow), runID, 1)

	var cancelled *rivertype.JobCancelError
	require.ErrorAs(t, err, &cancelled)
	r := h.run(runID)
	assert.Equal(t, automationrun.StatusFailed, r.status)
	assert.Equal(t, automationrun.FailureKindInvalidBody, *r.kind)
	assert.Empty(t, h.rcv.received(), "nothing is sent")
}

func TestHTTPStepRateLimitSnoozesWithoutSpendingAnAttempt(t *testing.T) {
	h := newHTTPStepHarness(t)
	runID := int64(fixtures.AutomationRunQuahogHookID)
	h.rcv.respond(http.StatusTooManyRequests, "slow down", "120")

	// Even the last attempt is not failed by a 429: it snoozes.
	err := h.attempt(h.worker(h.rcv.allow), runID, stepping.RunHTTPStepArgs{}.InsertOpts().MaxAttempts)

	var snooze *rivertype.JobSnoozeError
	require.ErrorAs(t, err, &snooze)
	assert.Equal(t, 120*time.Second, snooze.Duration)
	assert.Equal(t, automationrun.StatusActive, h.run(runID).status)
}

func TestHTTPStepRateLimitEndsAtTheRetryHorizon(t *testing.T) {
	h := newHTTPStepHarness(t)
	runID := int64(fixtures.AutomationRunQuahogHookID)
	h.rcv.respond(http.StatusTooManyRequests, "", "")
	w := h.worker(h.rcv.allow)
	h.clock = h.clock.Add(webhook.HookRetryHorizon) // the job was created a horizon ago

	err := w.Work(h.workCtx(), &river.Job[stepping.RunHTTPStepArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3, CreatedAt: h.clock.Add(-webhook.HookRetryHorizon)},
		Args:   stepping.RunHTTPStepArgs{RunID: runID},
	})

	var cancelled *rivertype.JobCancelError
	require.ErrorAs(t, err, &cancelled)
	r := h.run(runID)
	assert.Equal(t, automationrun.StatusFailed, r.status)
	assert.Equal(t, http.StatusTooManyRequests, *r.httpStatus)
	assert.Equal(t, automationrun.FailureKindHTTPStatus, *r.kind)
}

func TestHTTPStepOfAnErasedContactSendsNothing(t *testing.T) {
	h := newHTTPStepHarness(t)
	runID := int64(fixtures.AutomationRunQuahogHookID)
	h.DB.Contact.DeleteOneID(fixtures.ContactQuahogQuinnID).ExecX(context.Background())

	require.NoError(t, h.attempt(h.worker(h.rcv.allow), runID, 1))
	assert.Empty(t, h.rcv.received())

	require.NoError(t, h.attempt(h.worker(h.rcv.allow), 424242, 1), "an erased run ends the job too")
}

func TestHTTPStepRunsOnAQueueOfItsOwn(t *testing.T) {
	h := newHTTPStepHarness(t)
	runID := int64(fixtures.AutomationRunQuahogHookID)

	// The RunStep job parks the Enrollment on the HTTP job, queued apart from the Webhook
	// deliveries, so a slow receiver can only hold its own workers.
	queueStep(t, h.TestEnv, runID)
	require.True(t, h.RunNextStep(t, h.stepper()))

	listed := h.JobRowsOf(t, jobkind.HTTPStep.Kind)
	require.Len(t, listed, 1)
	queued := listed[0]
	assert.Equal(t, stepping.QueueHTTPSteps, queued.Queue)
	assert.NotEqual(t, jobs.QueueWebhooks, queued.Queue)
	assert.Equal(t, webhook.HookMaxAttempts(), queued.MaxAttempts)

	next := h.worker(h.rcv.allow).NextRetry(&river.Job[stepping.RunHTTPStepArgs]{JobRow: &rivertype.JobRow{Attempt: 2, AttemptedAt: &h.clock}})
	assert.Equal(t, webhook.HookNextRetry(h.clock, 2), next)
}

func TestHTTPStepOnAnExitedRunCallsNothingAndQueuesNothing(t *testing.T) {
	h := newHTTPStepHarness(t)
	runID := int64(fixtures.AutomationRunQuahogHookID)
	require.NoError(t, automations.ExitRun(context.Background(), h.DB.Scoped(fixtures.QuahogID), h.Publisher(), runID, time.Now()))

	require.NoError(t, h.attempt(h.worker(h.rcv.allow), runID, 1))

	assert.Empty(t, h.rcv.received(), "no call after an exit")
	r := h.run(runID)
	assert.Equal(t, automationrun.StatusExited, r.status)
	assert.Equal(t, 0, r.step)
	assert.Empty(t, queuedSteps(t, h.TestEnv))
}

// A step job delivered again while its HTTP step is still in flight finds the Enrollment where it
// was (an HTTP step makes no move): the call is queued once, so the receiver is not called twice.
func TestAStepJobExecutedTwiceOnAnHTTPStepQueuesTheCallOnce(t *testing.T) {
	h := newHTTPStepHarness(t)
	runID := int64(fixtures.AutomationRunQuahogHookID)
	h.reachHTTPStep(t, runID)

	require.NoError(t, h.stepper().Step(t.Context(), h.DB.Scoped(fixtures.QuahogID), runID, 0))

	assert.Equal(t, []map[string]any{{"run_id": float64(runID), "step": float64(0)}}, h.JobsOf(t, stepping.RunHTTPStepArgs{}.Kind()))
}
