package jobs_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivershared/riversharedtest"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/webhook"
)

func TestHookRetryPolicyLastAttemptFallsAboutThreeDaysAfterTheFirst(t *testing.T) {
	var elapsed time.Duration
	for attempt := 1; attempt < webhook.HookMaxAttempts(); attempt++ {
		elapsed += webhook.HookRetryDelay(attempt)
	}
	assert.GreaterOrEqual(t, elapsed, webhook.HookRetryHorizon)
	assert.Less(t, elapsed, webhook.HookRetryHorizon+webhook.HookRetryMaxDelay)
	assert.Equal(t, webhook.HookRetryFirstDelay, webhook.HookRetryDelay(1))
	assert.Equal(t, webhook.HookRetryMaxDelay, webhook.HookRetryDelay(webhook.HookMaxAttempts()))
}

func TestHookSnoozeReadsRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	assert.Equal(t, 120*time.Second, webhook.HookSnooze("120", now))
	assert.Equal(t, 5*time.Minute, webhook.HookSnooze(httpDate(now.Add(5*time.Minute)), now))
	assert.Equal(t, webhook.HookSnoozeDefault, webhook.HookSnooze("", now))
	assert.Equal(t, webhook.HookSnoozeDefault, webhook.HookSnooze("soon", now))
	assert.Equal(t, webhook.HookSnoozeDefault, webhook.HookSnooze(httpDate(now.Add(-time.Hour)), now))
	assert.Equal(t, webhook.HookSnoozeMax, webhook.HookSnooze("999999999", now))
}

func httpDate(at time.Time) string { return at.UTC().Format(http.TimeFormat) }

// newRetryReceiver answers every delivery with a status and an optional Retry-After, and
// returns the receiver's URL with the allowlist that admits it.
func newRetryReceiver(t *testing.T, status int, retryAfter string) (string, webhook.Allowlist) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	allow, err := webhook.ParseAllowlist(u.Host)
	require.NoError(t, err)
	return srv.URL, allow
}

// hookRetryHarness works a delivery job through river's own executor (rivertest), so the job
// row it leaves behind (state, attempt, scheduled_at) is what river really stored. river's
// clock and the worker's are the same stub.
type hookRetryHarness struct {
	t      *testing.T
	env    *riverEnv
	clock  *riversharedtest.TimeStub
	worker *rivertest.Worker[jobs.DeliverWebhookArgs, *sql.Tx]
	tx     *sql.Tx
	now    time.Time
}

func newHookRetryHarness(t *testing.T, status int, retryAfter string) *hookRetryHarness {
	t.Helper()
	return newHookRetryHarnessAt(t, status, retryAfter, time.Now().UTC().Truncate(time.Second))
}

// newHookRetryHarnessAt fixes the harness clock to now, for a test whose Retry-After is a date:
// it is built from the same instant the worker reads, however long the harness takes to set up.
func newHookRetryHarnessAt(t *testing.T, status int, retryAfter string, now time.Time) *hookRetryHarness {
	t.Helper()
	env := newRiverEnv(t)
	ctx := context.Background()
	receiverURL, allow := newRetryReceiver(t, status, retryAfter)
	_, err := env.DB.Scoped(fixtures.AcmeID).WebhookEndpoint().UpdateOneID(fixtures.WebhookCodebasicsID).SetURL(receiverURL).Save(ctx)
	require.NoError(t, err)

	h := &hookRetryHarness{t: t, env: env, clock: &riversharedtest.TimeStub{}, now: now}
	h.clock.StubNow(h.now)
	w := jobs.NewDeliverWebhookWorker(env.DB, env.cipher, webhook.NewClientAllowing(2*time.Second, allow), h.clock.Now, env.Bus, env.SystemMail)
	h.worker = rivertest.NewWorker(t, riverdatabasesql.New(env.pool), &river.Config{Test: river.TestConfig{Time: h.clock}}, river.Worker[jobs.DeliverWebhookArgs](w))
	h.tx, err = env.pool.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.tx.Rollback() })
	return h
}

// work inserts a delivery job with river's real client, then works it: the error is the
// worker's own (a failed delivery), the result what river stored.
func (h *hookRetryHarness) work(maxAttempts int) (*rivertest.WorkResult, error) {
	h.t.Helper()
	inserted, err := h.env.client.River().InsertTx(context.Background(), h.tx, jobs.DeliverWebhookArgs{
		EndpointID: fixtures.WebhookCodebasicsID, EventName: "email.opened", DeliveryID: "msg_1", Body: []byte(`{}`),
	}, &river.InsertOpts{MaxAttempts: maxAttempts})
	require.NoError(h.t, err)
	// The clock moves only after the insert, so a test can age the job.
	h.clock.StubNow(h.now)
	return h.worker.WorkJob(context.Background(), h.t, h.tx, inserted.Job)
}

func (h *hookRetryHarness) endpoint() *ent.WebhookEndpoint {
	return h.env.DB.Scoped(fixtures.AcmeID).WebhookEndpoint().Query().Where(webhookendpoint.ID(fixtures.WebhookCodebasicsID)).OnlyX(context.Background())
}

func TestDeliverWebhookRetriesOnTheHookPolicy(t *testing.T) {
	h := newHookRetryHarness(t, http.StatusServiceUnavailable, "")

	res, err := h.work(webhook.HookMaxAttempts())

	require.Error(t, err)
	assert.Equal(t, rivertype.JobStateRetryable, res.Job.State)
	assert.Equal(t, 1, res.Job.Attempt)
	assert.WithinDuration(t, h.now.Add(webhook.HookRetryDelay(1)), res.Job.ScheduledAt, time.Second)
	assert.Equal(t, 1, *h.endpoint().ConsecutiveFailures)
}

func TestDeliverWebhookFinalFailureEndsTheJob(t *testing.T) {
	h := newHookRetryHarness(t, http.StatusServiceUnavailable, "")

	res, err := h.work(1)

	require.Error(t, err)
	assert.Equal(t, rivertype.JobStateDiscarded, res.Job.State)
}

func TestDeliverWebhookRateLimitSnoozesWithoutSpendingAnAttempt(t *testing.T) {
	h := newHookRetryHarness(t, http.StatusTooManyRequests, "120")

	res, err := h.work(webhook.HookMaxAttempts())
	require.NoError(t, err)

	assert.Equal(t, rivertype.JobStateScheduled, res.Job.State)
	assert.Zero(t, res.Job.Attempt, "a snooze gives the attempt back")
	assert.WithinDuration(t, h.now.Add(120*time.Second), res.Job.ScheduledAt, time.Second)
	e := h.endpoint()
	assert.Nil(t, e.ConsecutiveFailures, "a 429 is not a failure")
	assert.Nil(t, e.LastErrorKind)
	assert.Nil(t, e.DisabledAt)
}

func TestDeliverWebhookRateLimitLeavesAFailingRunAsItWas(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	h := newHookRetryHarnessAt(t, http.StatusTooManyRequests, httpDate(now.Add(10*time.Minute)), now)
	_, err := h.env.DB.Scoped(fixtures.AcmeID).WebhookEndpoint().UpdateOneID(fixtures.WebhookCodebasicsID).
		SetConsecutiveFailures(3).SetFailingSince(h.now.Add(-time.Hour)).Save(context.Background())
	require.NoError(t, err)

	res, err := h.work(webhook.HookMaxAttempts())
	require.NoError(t, err)

	assert.Equal(t, rivertype.JobStateScheduled, res.Job.State)
	assert.WithinDuration(t, h.now.Add(10*time.Minute), res.Job.ScheduledAt, 2*time.Second, "an HTTP-date Retry-After")
	assert.Equal(t, 3, *h.endpoint().ConsecutiveFailures)
}

func TestDeliverWebhookRateLimitWithoutRetryAfterUsesTheDefault(t *testing.T) {
	h := newHookRetryHarness(t, http.StatusTooManyRequests, "")

	res, err := h.work(webhook.HookMaxAttempts())
	require.NoError(t, err)

	assert.WithinDuration(t, h.now.Add(webhook.HookSnoozeDefault), res.Job.ScheduledAt, time.Second)
}

func TestDeliverWebhookRateLimitEndsTheJobPastTheHorizon(t *testing.T) {
	h := newHookRetryHarness(t, http.StatusTooManyRequests, "1")
	h.now = h.now.Add(webhook.HookRetryHorizon + time.Hour)

	res, err := h.work(webhook.HookMaxAttempts())
	require.NoError(t, err)

	assert.Equal(t, rivertype.JobStateCancelled, res.Job.State)
	e := h.endpoint()
	assert.Nil(t, e.ConsecutiveFailures, "giving up on a rate limited delivery is not a failure of the endpoint")
	assert.Nil(t, e.DisabledAt)
}

func TestDeliverWebhookGoneDisablesTheEndpointAndTheJobEnds(t *testing.T) {
	h := newHookRetryHarness(t, http.StatusGone, "")

	res, err := h.work(webhook.HookMaxAttempts())
	require.Error(t, err)
	require.Equal(t, rivertype.JobStateRetryable, res.Job.State)
	assert.NotNil(t, h.endpoint().DisabledAt)

	// The retry finds the endpoint disabled and drops the delivery.
	again, err := h.worker.WorkJob(context.Background(), h.t, h.tx, res.Job)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateCompleted, again.Job.State)
}
