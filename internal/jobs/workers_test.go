package jobs_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"
	"github.com/samber/lo"
	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	appdb "github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/domainverify"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/registry"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/webhook"
)

// riverEnv is a real river client over the test database. The workers read and
// write through the test's txdb-bound ent client (rolled back with the test), while
// the queue itself is real: river inserts commit on their own pool, so the test
// clears river_job before and after.
type riverEnv struct {
	*testhelper.TestEnv
	pool   *sql.DB
	client *jobs.Client
	cipher *secrets.Cipher
	cfg    *config.Config
	sender *fakeSender
}

func newRiverEnv(t *testing.T) *riverEnv {
	t.Helper()
	env := testhelper.Setup(t)

	cfg, err := config.Load("test")
	require.NoError(t, err)
	pool, err := appdb.OpenRiverDB(cfg.DatabaseURL, cfg.DBPool)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	fs := &fakeSender{}
	mod := newMod(env, resolvingTo(fs))
	client, err := jobs.NewClient(pool, env.DB, mod, cipher, env.Bus, env.SystemMail, nil, nil, registry.Default(), cfg.AppURL, cfg.OutboundAllow, jobs.Retention{OutboxFloor: cfg.OutboxFloor, Events: cfg.EventsRetention, UnverifiedUsers: cfg.UnverifiedUserRetention})
	require.NoError(t, err)
	e := &riverEnv{TestEnv: env, pool: pool, client: client, cipher: cipher, cfg: cfg, sender: fs}
	e.clearQueue(t)
	t.Cleanup(func() { e.clearQueue(t) })
	return e
}

// workCtx is the context river hands a worker: it carries the client the fan-out
// workers insert follow-up jobs with.
func (e *riverEnv) workCtx() context.Context {
	return rivertest.WorkContext(context.Background(), e.client.River())
}

// clearQueue removes every job from the real queue.
func (e *riverEnv) clearQueue(t *testing.T) {
	t.Helper()
	_, err := e.client.River().JobDeleteMany(context.Background(), river.NewJobDeleteManyParams().Queues(river.QueueDefault, broadcasts.Queue, jobs.QueueWebhooks, jobs.QueueHTTPSteps))
	require.NoError(t, err)
}

// queued returns the kinds of the jobs currently in the queue, oldest first.
func (e *riverEnv) queued(t *testing.T) []string {
	t.Helper()
	res, err := e.client.River().JobList(context.Background(),
		river.NewJobListParams().OrderBy(river.JobListOrderByID, river.SortOrderAsc).First(1000))
	require.NoError(t, err)
	kinds := make([]string, 0, len(res.Jobs))
	for _, j := range res.Jobs {
		kinds = append(kinds, j.Kind)
	}
	return kinds
}

func job[T river.JobArgs](args T) *river.Job[T] {
	return &river.Job[T]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3}, Args: args}
}

func TestClientEnqueuesEveryJobKind(t *testing.T) {
	e := newRiverEnv(t)
	ctx := context.Background()

	require.NoError(t, e.client.EnqueueWelcome(ctx, "new@example.com", "New"))
	require.NoError(t, e.client.EnqueuePasswordReset(ctx, "a@example.com", "tok", true))
	require.NoError(t, e.client.EnqueueEmailVerification(ctx, "a@example.com", "tok", true))
	require.NoError(t, e.client.EnqueueEmailChangeConfirm(ctx, "a@example.com", "tok", true))
	require.NoError(t, e.client.EnqueueMemberInvite(ctx, "a@example.com", "https://x/invite", "Acme", "Jane"))
	require.NoError(t, e.client.EnqueueSendingDomainVerify(ctx, fixtures.SendingDomainVerifiedID))
	require.NoError(t, e.client.EnqueueSecondFactorRequired(ctx, fixtures.UmbrellaID))
	require.NoError(t, e.client.OnEvent(ctx, fixtures.AcmeID, fixtures.ContactAliceID, "contact.created"))

	assert.Equal(t, []string{
		"send_welcome",
		"send_auth_mail", "send_auth_mail", "send_auth_mail",
		"send_member_invite", "sending_domain_verify", "second_factor_required_notify",
		"automation_evaluate_trigger",
	}, e.queued(t))
}

func TestClientStartAndStop(t *testing.T) {
	e := newRiverEnv(t)
	ctx := context.Background()
	require.NoError(t, e.client.Start(ctx))
	require.NoError(t, e.client.Stop(ctx))
}

// A real river runtime picks the job up and runs the registered worker.
func TestClientRunsAnEnqueuedWelcomeJob(t *testing.T) {
	e := newRiverEnv(t)
	ctx := context.Background()
	require.NoError(t, e.client.Start(ctx))
	t.Cleanup(func() { _ = e.client.Stop(context.Background()) })

	require.NoError(t, e.client.EnqueueWelcome(ctx, "runtime@example.com", "Run"))

	require.Eventually(t, func() bool {
		for _, m := range e.SystemMail.Messages() {
			if m.To == "runtime@example.com" {
				return true
			}
		}
		return false
	}, 10*time.Second, 50*time.Millisecond)
}

func TestDispatchFansOutToMatchingEnabledEndpoints(t *testing.T) {
	e := newRiverEnv(t)
	ctx := context.Background()

	// Endpoint 100 filters to three event types; 101 is disabled.
	require.NoError(t, e.client.Dispatch(ctx, e.DB.Scoped(fixtures.AcmeID), "email.opened", "d-1", []byte(`{}`)))
	assert.Equal(t, []string{"deliver_webhook"}, e.queued(t))

	// Filtered out: no endpoint of the workspace subscribes to it.
	require.NoError(t, e.client.Dispatch(ctx, e.DB.Scoped(fixtures.AcmeID), "segment.created", "d-2", []byte(`{}`)))
	assert.Len(t, e.queued(t), 1)

	// An endpoint with no filter receives every event (Globex's, workspace 2).
	require.NoError(t, e.client.Dispatch(ctx, e.DB.Scoped(fixtures.GlobexID), "segment.created", "d-3", []byte(`{}`)))
	assert.Len(t, e.queued(t), 2)
}

func TestWelcomeWorker(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, jobs.NewWelcomeWorker(env.SystemMail).Work(context.Background(),
		job(jobs.SendWelcomeArgs{Email: "w@example.com", Name: "Wendy"})))
	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "w@example.com", msgs[0].To)
	assert.Contains(t, msgs[0].Text, "Wendy")

	err := jobs.NewWelcomeWorker(nil).Work(context.Background(), job(jobs.SendWelcomeArgs{Email: "w@example.com"}))
	require.Error(t, err, "no system sender configured")
}

func TestAuthMailWorker(t *testing.T) {
	env := testhelper.Setup(t)
	w := jobs.NewAuthMailWorker(env.SystemMail, "https://app.example/")
	for _, flow := range []struct{ name, path string }{
		{"password_reset", "/reset-password"},
		{"email_verify", "/verify-email"},
		{"email_change", "/confirm-email-change"},
	} {
		require.NoError(t, w.Work(context.Background(), job(jobs.SendAuthMailArgs{Flow: flow.name, Email: "a@example.com", Token: "t/1"})))
		msgs := env.SystemMail.Messages()
		assert.Contains(t, msgs[len(msgs)-1].Text, "https://app.example"+flow.path+"?token=t%2F1")
	}

	require.Error(t, w.Work(context.Background(), job(jobs.SendAuthMailArgs{Flow: "nope", Email: "a@example.com"})))
	require.Error(t, jobs.NewAuthMailWorker(nil, "").Work(context.Background(), job(jobs.SendAuthMailArgs{Flow: "password_reset"})))
}

// The Taken-address notice (ADR 0031) carries no token: only the sign-in and the
// forgot-password pages, so it can never be a way into the account.
func TestAuthMailWorkerAddressTakenNoticeCarriesNoToken(t *testing.T) {
	env := testhelper.Setup(t)
	w := jobs.NewAuthMailWorker(env.SystemMail, "https://app.example/")

	require.NoError(t, w.Work(context.Background(), job(jobs.SendAuthMailArgs{Flow: "address_taken", Email: "owner@example.com"})))

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "owner@example.com", msgs[0].To)
	assert.NotContains(t, msgs[0].Text, "token")
	assert.Contains(t, msgs[0].Text, "https://app.example/login")
	assert.Contains(t, msgs[0].Text, "https://app.example/forgot-password")
}

// A Discard job does the whole job (builds the mail) but sends nothing: forgot-password
// enqueues one for an unknown or over-limit address so every request costs the same.
func TestAuthMailWorkerDiscardSendsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	w := jobs.NewAuthMailWorker(env.SystemMail, "https://app.example/")
	require.NoError(t, w.Work(context.Background(), job(jobs.SendAuthMailArgs{Flow: "password_reset", Email: "a@example.com", Token: "t", Discard: true})))
	assert.Empty(t, env.SystemMail.Messages())
	require.Error(t, w.Work(context.Background(), job(jobs.SendAuthMailArgs{Flow: "nope", Email: "a@example.com", Discard: true})), "a discarded job still validates what it renders")
}

func TestMemberInviteWorker(t *testing.T) {
	env := testhelper.Setup(t)
	w := jobs.NewMemberInviteWorker(env.SystemMail)
	require.NoError(t, w.Work(context.Background(), job(jobs.SendMemberInviteArgs{
		Email: "i@example.com", InviteURL: "https://x/invite/1", WorkspaceName: "Acme", InviterName: "Jane",
	})))
	// An invite without an inviter name falls back to a generic subject.
	require.NoError(t, w.Work(context.Background(), job(jobs.SendMemberInviteArgs{
		Email: "i@example.com", InviteURL: "https://x/invite/2", WorkspaceName: "Acme",
	})))
	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 2)
	assert.Contains(t, msgs[0].Text, "Jane invited you")
	assert.Contains(t, msgs[1].Text, "Someone invited you")
	assert.Equal(t, "You're invited to Acme on sphericon", msgs[0].Subject)

	require.Error(t, jobs.NewMemberInviteWorker(nil).Work(context.Background(), job(jobs.SendMemberInviteArgs{})))
}

// webhookReceiver is an httptest server the Instance allowlist permits: the real hardened
// client reaches it, and it records what it got and answers with the status set.
type webhookReceiver struct {
	srv    *httptest.Server
	allow  webhook.Allowlist
	mu     sync.Mutex
	status int
	bodies []string
	verify []error
}

func newWebhookReceiver(t *testing.T) *webhookReceiver {
	t.Helper()
	r := &webhookReceiver{status: http.StatusOK}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		wh, _ := standardwebhooks.NewWebhook(webhookSecret)
		r.mu.Lock()
		defer r.mu.Unlock()
		r.bodies = append(r.bodies, string(body))
		r.verify = append(r.verify, wh.Verify(body, req.Header))
		w.WriteHeader(r.status)
	}))
	t.Cleanup(r.srv.Close)
	u, err := url.Parse(r.srv.URL)
	require.NoError(t, err)
	r.allow, err = webhook.ParseAllowlist(u.Host)
	require.NoError(t, err)
	return r
}

func (r *webhookReceiver) setStatus(status int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = status
}

func (r *webhookReceiver) received() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.bodies)
}

// webhookSecret is the signing secret of the WebhookCodebasics fixture.
const webhookSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

func TestDeliverWebhookWorker(t *testing.T) {
	env := testhelper.Setup(t)
	cfg, err := config.Load("test")
	require.NoError(t, err)
	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	ctx := context.Background()
	args := func(id int64) jobs.DeliverWebhookArgs {
		return jobs.DeliverWebhookArgs{EndpointID: id, EventName: "email.opened", DeliveryID: "msg_1", Body: []byte(`{"a":1}`)}
	}
	rcv := newWebhookReceiver(t)
	worker := func(allow webhook.Allowlist) *jobs.DeliverWebhookWorker {
		return jobs.NewDeliverWebhookWorker(env.DB, cipher, webhook.NewClientAllowing(2*time.Second, allow), time.Now, env.Bus, env.SystemMail)
	}
	ws := env.DB.Scoped(fixtures.AcmeID)
	_, err = ws.WebhookEndpoint().UpdateOneID(fixtures.WebhookCodebasicsID).SetURL(rcv.srv.URL).Save(ctx)
	require.NoError(t, err)

	// A delivery reaches the receiver, which verifies the signature with the endpoint secret.
	require.NoError(t, worker(rcv.allow).Work(ctx, job(args(fixtures.WebhookCodebasicsID))))
	require.Len(t, rcv.received(), 1)
	assert.JSONEq(t, `{"a":1}`, rcv.received()[0])
	assert.NoError(t, rcv.verify[0])

	// Deleted or disabled since enqueue: dropped without a request.
	require.NoError(t, worker(rcv.allow).Work(ctx, job(args(424242))))
	require.NoError(t, worker(rcv.allow).Work(ctx, job(args(fixtures.WebhookCodebasicsDisabledID))))
	assert.Len(t, rcv.received(), 1)

	// A non-2xx answer surfaces so river retries.
	rcv.mu.Lock()
	rcv.status = http.StatusInternalServerError
	rcv.mu.Unlock()
	require.Error(t, worker(rcv.allow).Work(ctx, job(args(fixtures.WebhookCodebasicsID))))
	assert.Len(t, rcv.received(), 2)

	// With the allowlist unset the loopback receiver is refused and gets nothing.
	require.Error(t, worker(webhook.Allowlist{}).Work(ctx, job(args(fixtures.WebhookCodebasicsID))))
	assert.Len(t, rcv.received(), 2)
}

// A failed delivery is remembered on the endpoint with a closed error kind, the run of
// failures keeps its start, and a 2xx clears it all. The clock is injected.
func TestDeliverWebhookWorkerRemembersFailures(t *testing.T) {
	env := testhelper.Setup(t)
	cfg, err := config.Load("test")
	require.NoError(t, err)
	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	ctx := context.Background()
	rcv := newWebhookReceiver(t)
	ws := env.DB.Scoped(fixtures.AcmeID)
	_, err = ws.WebhookEndpoint().UpdateOneID(fixtures.WebhookCodebasicsID).SetURL(rcv.srv.URL).Save(ctx)
	require.NoError(t, err)

	clock := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	worker := func(allow webhook.Allowlist, timeout time.Duration) *jobs.DeliverWebhookWorker {
		return jobs.NewDeliverWebhookWorker(env.DB, cipher, webhook.NewClientAllowing(timeout, allow), func() time.Time { return clock }, env.Bus, env.SystemMail)
	}
	deliver := func(w *jobs.DeliverWebhookWorker) error {
		return w.Work(ctx, job(jobs.DeliverWebhookArgs{EndpointID: fixtures.WebhookCodebasicsID, EventName: "email.opened", DeliveryID: "msg_1", Body: []byte(`{}`)}))
	}
	state := func() *ent.WebhookEndpoint {
		return ws.WebhookEndpoint().Query().Where(webhookendpoint.ID(fixtures.WebhookCodebasicsID)).OnlyX(ctx)
	}

	// A healthy endpoint carries no failure state.
	require.NoError(t, deliver(worker(rcv.allow, 2*time.Second)))
	assert.Nil(t, state().ConsecutiveFailures)

	// A non-2xx answer: the run begins, with the status.
	rcv.setStatus(http.StatusServiceUnavailable)
	require.Error(t, deliver(worker(rcv.allow, 2*time.Second)))
	e := state()
	require.NotNil(t, e.ConsecutiveFailures)
	assert.Equal(t, 1, *e.ConsecutiveFailures)
	assert.True(t, clock.Equal(*e.FailingSince))
	assert.True(t, clock.Equal(*e.LastFailureAt))
	assert.Equal(t, webhookendpoint.LastErrorKindHTTPStatus, *e.LastErrorKind)
	assert.Equal(t, http.StatusServiceUnavailable, *e.LastErrorStatus)
	assert.Nil(t, e.DisabledAt, "a failure alone does not disable (that is the disable rule's job)")

	// A later failure of another kind extends the run, which keeps its start.
	began := clock
	clock = clock.Add(90 * time.Minute)
	require.Error(t, deliver(worker(webhook.Allowlist{}, 2*time.Second)))
	e = state()
	assert.Equal(t, 2, *e.ConsecutiveFailures)
	assert.True(t, began.Equal(*e.FailingSince))
	assert.True(t, clock.Equal(*e.LastFailureAt))
	assert.Equal(t, webhookendpoint.LastErrorKindBlockedAddress, *e.LastErrorKind)
	assert.Nil(t, e.LastErrorStatus)

	// A receiver that does not answer in time is a timeout.
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(slow.Close)
	t.Cleanup(func() { close(release) })
	_, err = ws.WebhookEndpoint().UpdateOneID(fixtures.WebhookCodebasicsID).SetURL(slow.URL).Save(ctx)
	require.NoError(t, err)
	slowURL, err := url.Parse(slow.URL)
	require.NoError(t, err)
	slowAllow, err := webhook.ParseAllowlist(slowURL.Host)
	require.NoError(t, err)
	require.Error(t, deliver(worker(slowAllow, 50*time.Millisecond)))
	assert.Equal(t, webhookendpoint.LastErrorKindTimeout, *state().LastErrorKind)

	// A 2xx resets the state.
	_, err = ws.WebhookEndpoint().UpdateOneID(fixtures.WebhookCodebasicsID).SetURL(rcv.srv.URL).Save(ctx)
	require.NoError(t, err)
	rcv.setStatus(http.StatusOK)
	require.NoError(t, deliver(worker(rcv.allow, 2*time.Second)))
	e = state()
	assert.Nil(t, e.ConsecutiveFailures)
	assert.Nil(t, e.FailingSince)
	assert.Nil(t, e.LastErrorKind)
	assert.Nil(t, e.LastErrorStatus)
	assert.Nil(t, e.LastFailureAt)
}

// disableHarness delivers to the WebhookCodebasics endpoint pointed at a receiver the test
// controls, with an injected clock, and reads back what a delivery left behind.
type disableHarness struct {
	t     *testing.T
	env   *testhelper.TestEnv
	rcv   *webhookReceiver
	clock time.Time
	w     *jobs.DeliverWebhookWorker
}

func newDisableHarness(t *testing.T) *disableHarness {
	t.Helper()
	env := testhelper.Setup(t)
	cfg, err := config.Load("test")
	require.NoError(t, err)
	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	h := &disableHarness{t: t, env: env, rcv: newWebhookReceiver(t), clock: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	_, err = env.DB.Scoped(fixtures.AcmeID).WebhookEndpoint().UpdateOneID(fixtures.WebhookCodebasicsID).SetURL(h.rcv.srv.URL).Save(context.Background())
	require.NoError(t, err)
	h.w = jobs.NewDeliverWebhookWorker(env.DB, cipher, webhook.NewClientAllowing(2*time.Second, h.rcv.allow), func() time.Time { return h.clock }, env.Bus, env.SystemMail)
	return h
}

func (h *disableHarness) deliver() error {
	return h.w.Work(context.Background(), job(jobs.DeliverWebhookArgs{
		EndpointID: fixtures.WebhookCodebasicsID, EventName: "email.opened", DeliveryID: "msg_1", Body: []byte(`{}`),
	}))
}

func (h *disableHarness) endpoint() *ent.WebhookEndpoint {
	return h.env.DB.Scoped(fixtures.AcmeID).WebhookEndpoint().Query().Where(webhookendpoint.ID(fixtures.WebhookCodebasicsID)).OnlyX(context.Background())
}

// A 410 retires the endpoint on that delivery, whatever the run of failures so far.
func TestDeliverWebhookWorkerDisablesOnGone(t *testing.T) {
	h := newDisableHarness(t)
	ctx := context.Background()
	owners, err := h.env.DB.Scoped(fixtures.AcmeID).Membership().Query().Where(membership.RoleEQ(membership.RoleOwner)).WithUser().All(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, owners)

	h.rcv.setStatus(http.StatusGone)
	require.Error(t, h.deliver(), "river still sees the failed delivery")

	e := h.endpoint()
	assert.NotNil(t, e.DisabledAt)
	require.NotNil(t, e.DisabledReason)
	assert.Equal(t, webhookendpoint.DisabledReasonGone, *e.DisabledReason)

	// One mail to each owner, through the platform sender; the notice names the endpoint's origin only.
	msgs := h.env.SystemMail.Messages()
	assert.Len(t, msgs, len(owners))
	for _, m := range msgs {
		assert.Contains(t, m.Subject, "webhook")
		assert.Contains(t, m.Text, "410")
		assert.Contains(t, m.Text, h.rcv.srv.URL)
	}

	// The Audit entry is the system actor's, and is the only one: the counter writes are not audited.
	entries := lo.FilterMap(h.env.OutboxEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) (*events.AuditEntry, bool) {
		a := ev.(*events.AuditEntry)
		return a, a.Action == "webhook_endpoint.update"
	})
	require.Len(t, entries, 1)
	assert.Equal(t, events.ActorSystem, entries[0].Actor.Kind)
	assert.Contains(t, entries[0].Diff, "disabled_at")

	// A delivery that was already enqueued finds the endpoint off: no request, no second mail.
	before := len(h.rcv.received())
	require.NoError(t, h.deliver())
	assert.Len(t, h.rcv.received(), before)
	assert.Len(t, h.env.SystemMail.Messages(), len(owners))
}

func TestVerifySendingDomainWorker(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	gone := dnstest.Resolver(t, nil).LookupTXT

	// Verified domain whose DNS vanished: flips, owner is emailed.
	w := jobs.NewVerifySendingDomainWorker(domainverify.New(env.DB, gone, nil, accounts.OwnerNotifier(env.DB, env.SystemMail)))
	require.NoError(t, w.Work(ctx, job(jobs.VerifySendingDomainArgs{SendingDomainID: fixtures.SendingDomainVerifiedID})))
	assert.Len(t, env.SystemMail.Messages(), 1)

	// A failing alert never fails the verification.
	env.SystemMail.SetErr(errors.New("smtp down"))
	require.NoError(t, jobs.NewVerifySendingDomainWorker(domainverify.New(env.DB, gone, nil, accounts.OwnerNotifier(env.DB, env.SystemMail))).
		Work(ctx, job(jobs.VerifySendingDomainArgs{SendingDomainID: fixtures.SendingDomainVerifiedID})))

	// Unknown domain id is an error.
	require.Error(t, w.Work(ctx, job(jobs.VerifySendingDomainArgs{SendingDomainID: 424242})))
}

func TestRecheckSendingDomainsWorkerFansOut(t *testing.T) {
	e := newRiverEnv(t)
	total, err := e.DB.SendingDomain.Query().Where(sendingdomain.DiscardedAtIsNil()).Count(context.Background())
	require.NoError(t, err)

	require.NoError(t, jobs.NewRecheckSendingDomainsWorker(domainverify.New(e.DB, nil, nil, nil)).Work(e.workCtx(), job(jobs.RecheckSendingDomainsArgs{})))

	kinds := e.queued(t)
	assert.Len(t, kinds, total)
	for _, k := range kinds {
		assert.Equal(t, "sending_domain_verify", k)
	}
}

func TestPurgeAuthAttemptsWorkerRemovesStaleRowsOnly(t *testing.T) {
	e := newRiverEnv(t)
	ctx := e.workCtx()

	require.NoError(t, jobs.NewPurgeAuthAttemptsWorker(e.DB).Work(ctx, job(jobs.PurgeAuthAttemptsArgs{})))

	_, err := e.DB.AuthAttempt.Get(ctx, fixtures.StaleLoginAttemptID)
	assert.True(t, ent.IsNotFound(err), "the stale row is purged")
	_, err = e.DB.AuthAttempt.Get(ctx, fixtures.FreshLoginAttemptID)
	assert.NoError(t, err, "the current row stays")
}

func TestEvaluateTriggerAndRunStepWorkers(t *testing.T) {
	e := newRiverEnv(t)
	ctx := e.workCtx()

	c, err := e.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("wk@test.dev").SetFirstName("Wk").Save(ctx)
	require.NoError(t, err)
	_, err = testhelper.AutomationWithSteps(ctx, e.DB, e.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Flow").SetTriggerEvent("worker.test").SetStatus(automation.StatusActive), "["+emailStep+`,{"type":"wait","seconds":3600},`+emailStep+"]")
	require.NoError(t, err)

	require.NoError(t, jobs.NewEvaluateTriggerWorker(e.DB).Work(ctx, job(jobs.EvaluateTriggerArgs{
		WorkspaceID: fixtures.AcmeID, ContactID: c.ID, Action: "worker.test",
	})))
	require.Equal(t, []string{"automation_run_step"}, e.queued(t))
	run := e.DB.AutomationRun.Query().Where(automationrun.ContactID(c.ID)).OnlyX(ctx)

	// Step 1 sends and schedules the next step; the wait step schedules a delayed
	// one; the last step finishes the run without queueing more.
	w := jobs.NewRunStepWorker(e.DB, noFence, newMod(e.TestEnv, resolvingTo(e.sender)))
	require.NoError(t, w.Work(ctx, job(jobs.RunStepArgs{RunID: run.ID})))
	assert.Len(t, e.sender.sent, 1)
	assert.Len(t, e.queued(t), 2)
	require.NoError(t, w.Work(ctx, job(jobs.RunStepArgs{RunID: run.ID})))
	assert.Len(t, e.queued(t), 3, "the wait step queues a delayed follow-up")
	require.NoError(t, w.Work(ctx, job(jobs.RunStepArgs{RunID: run.ID})))
	assert.Len(t, e.sender.sent, 2)
	assert.Len(t, e.queued(t), 4)
	require.NoError(t, w.Work(ctx, job(jobs.RunStepArgs{RunID: run.ID})))
	assert.Equal(t, automationrun.StatusCompleted, e.DB.AutomationRun.GetX(ctx, run.ID).Status)
	assert.Len(t, e.queued(t), 4, "a finished run queues nothing")

	// An unknown run (erased with its Contact) is finished, not retried.
	require.NoError(t, w.Work(ctx, job(jobs.RunStepArgs{RunID: 424242})))
}

func TestBroadcastWorkers(t *testing.T) {
	e := newRiverEnv(t)
	ctx := e.workCtx()
	w := jobs.NewSendBroadcastWorker(e.DB, newMod(e.TestEnv, resolvingTo(e.sender)))
	assert.Equal(t, 10*time.Minute, w.Timeout(nil))

	// A draft (unscheduled after queueing) is not sent.
	require.NoError(t, w.Work(ctx, job(broadcasts.SendArgs{BroadcastID: fixtures.BroadcastDraftID})))
	require.Empty(t, e.queued(t))

	// Fan-out: one recipient job per eligible contact.
	e.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetStatus(broadcast.StatusSending).ExecX(ctx)
	require.NoError(t, w.Work(ctx, job(broadcasts.SendArgs{BroadcastID: fixtures.BroadcastDraftID})))
	kinds := e.queued(t)
	require.NotEmpty(t, kinds)
	for _, k := range kinds {
		assert.Equal(t, "send_broadcast_recipient", k)
	}
	recs := e.DB.BroadcastRecipient.Query().Where(broadcastrecipient.BroadcastID(fixtures.BroadcastDraftID)).AllX(ctx)
	require.Len(t, recs, len(kinds))

	// Delivering every recipient finalizes the broadcast.
	rw := jobs.NewSendRecipientWorker(e.DB, newMod(e.TestEnv, resolvingTo(e.sender)))
	for _, r := range recs {
		require.NoError(t, rw.Work(ctx, job(jobs.SendRecipientArgs{RecipientID: r.ID, BroadcastID: fixtures.BroadcastDraftID})))
	}
	assert.Len(t, e.sender.sent, len(recs))
	assert.Equal(t, broadcast.StatusSent, e.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Status)

	// An empty audience finalizes in the plan phase and queues nothing more.
	before := len(e.queued(t))
	require.NoError(t, w.Work(ctx, job(broadcasts.SendArgs{BroadcastID: fixtures.BroadcastEmptyAudienceID})))
	assert.Len(t, e.queued(t), before)

	// A suspended workspace holds: the job snoozes rather than fails.
	_, err := suspension.SuspendWorkspace(ctx, e.Bus, fixtures.AcmeID, suspension.System, "complaints")
	require.NoError(t, err)
	e.DB.Broadcast.UpdateOneID(fixtures.BroadcastProSegmentID).SetStatus(broadcast.StatusSending).ExecX(ctx)
	var snooze *river.JobSnoozeError
	require.ErrorAs(t, w.Work(ctx, job(broadcasts.SendArgs{BroadcastID: fixtures.BroadcastProSegmentID})), &snooze)

	// A resolver failure is a plain error: river retries it.
	hold := jobs.NewSendBroadcastWorker(e.DB, newMod(e.TestEnv, failingResolver(errors.New("no integration"))))
	require.Error(t, hold.Work(ctx, job(broadcasts.SendArgs{BroadcastID: fixtures.BroadcastProSegmentID})))
}

func TestSendRecipientWorkerRecordsTerminalFailure(t *testing.T) {
	e := newRiverEnv(t)
	ctx := e.workCtx()
	mod := newMod(e.TestEnv, resolvingTo(e.sender))
	ids, err := planAudience(ctx, e.DB, mod, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	require.NotEmpty(t, ids)

	failing := jobs.NewSendRecipientWorker(e.DB, newMod(e.TestEnv, resolvingTo(erroringSender{})))
	// Not the last attempt: the error surfaces for retry, row stays pending.
	err = failing.Work(ctx, job(jobs.SendRecipientArgs{RecipientID: ids[0], BroadcastID: fixtures.BroadcastDraftID}))
	require.Error(t, err)
	assert.True(t, stillPending(ctx, e.DB, ids[0]))

	// Last attempt: the row is marked failed so the broadcast can finalize.
	last := job(jobs.SendRecipientArgs{RecipientID: ids[0], BroadcastID: fixtures.BroadcastDraftID})
	last.Attempt = last.MaxAttempts
	require.Error(t, failing.Work(ctx, last))
	assert.False(t, stillPending(ctx, e.DB, ids[0]), "the last attempt fails the recipient so the broadcast can finalize")
}

// takeoverSender fails the send after another attempt took over the claim, as one that outlived
// its lease would find: the failure cannot release a claim that is no longer its own.
type takeoverSender struct{ db *ent.Client }

func (takeoverSender) DefaultFrom() (string, string) { return "hello@codebasics.dev", "CodeBasics" }

func (s takeoverSender) Send(ctx context.Context, _ messaging.EmailMessage) (messaging.Receipt, error) {
	s.db.OutboundMessage.Update().
		Where(outboundmessage.StatusEQ(outboundmessage.StatusPending)).
		SetClaimedAt(time.Now()).ExecX(ctx)
	return messaging.Receipt{}, errors.New("smtp unavailable")
}

// On the last attempt a recipient whose claim another attempt still holds is not failed: that
// attempt may yet reach the provider, so the job snoozes (which spends no attempt) and gives up
// once the lease is out.
func TestSendRecipientWorkerSnoozesInsteadOfGivingUpOnALiveClaim(t *testing.T) {
	e := newRiverEnv(t)
	ctx := e.workCtx()
	mod := newMod(e.TestEnv, resolvingTo(e.sender))
	ids, err := planAudience(ctx, e.DB, mod, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	require.NotEmpty(t, ids)

	worker := jobs.NewSendRecipientWorker(e.DB, newMod(e.TestEnv, resolvingTo(takeoverSender{db: e.DB})))
	last := job(jobs.SendRecipientArgs{RecipientID: ids[0], BroadcastID: fixtures.BroadcastDraftID})
	last.Attempt = last.MaxAttempts

	var snooze *river.JobSnoozeError
	require.ErrorAs(t, worker.Work(ctx, last), &snooze)

	assert.True(t, stillPending(ctx, e.DB, ids[0]), "nothing is failed while the claim is live")
	assert.Equal(t, broadcast.StatusSending, e.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Status)
}

func TestErrorHandlerLogsAndKeepsRetrying(t *testing.T) {
	var buf bytes.Buffer
	h := jobs.NewErrorHandler(slog.New(slog.NewTextHandler(&buf, nil)))
	row := &rivertype.JobRow{ID: 7, Kind: "send_welcome", Queue: "default", Attempt: 2, MaxAttempts: 5}

	res := h.HandleError(context.Background(), row, errors.New("boom"))
	require.NotNil(t, res)
	assert.False(t, res.SetCancelled, "river keeps following its retry schedule")
	assert.Contains(t, buf.String(), "river job errored")
	assert.Contains(t, buf.String(), "send_welcome")

	res = h.HandlePanic(context.Background(), row, "kaboom", "stack")
	require.NotNil(t, res)
	assert.False(t, res.SetCancelled)
	assert.Contains(t, buf.String(), "river job panicked")
	assert.Contains(t, buf.String(), "kaboom")
}

// A queued plan job that fires after its Broadcast has finished is stale: it finishes
// without an error (so river does not retry it) and leaves the status alone.
func TestSendBroadcastWorkerFinishesAStaleJob(t *testing.T) {
	e := newRiverEnv(t)
	ctx := e.workCtx()
	w := jobs.NewSendBroadcastWorker(e.DB, newMod(e.TestEnv, resolvingTo(e.sender)))

	for id, want := range map[int64]broadcast.Status{
		fixtures.BroadcastSentID:   broadcast.StatusSent,
		fixtures.BroadcastFailedID: broadcast.StatusFailed,
	} {
		require.NoError(t, w.Work(ctx, job(broadcasts.SendArgs{BroadcastID: id})), "broadcast %d", id)
		assert.Equal(t, want, e.DB.Broadcast.GetX(ctx, id).Status, "broadcast %d", id)
	}
	assert.Empty(t, e.queued(t))
}

func TestRecheckTrackingDomainsWorkerFansOutOneJobPerServingOrDNSVerifiedHost(t *testing.T) {
	e := newRiverEnv(t)
	ctx := e.workCtx()
	due, err := e.DB.SendingDomain.Query().
		Where(sendingdomain.TrackingStateIn(sendingdomain.TrackingStateDNSVerified, sendingdomain.TrackingStateServing), sendingdomain.IDNEQ(fixtures.SendingDomainArchivedID)).
		Count(ctx)
	require.NoError(t, err)

	require.NoError(t, jobs.NewRecheckTrackingDomainsWorker(domainverify.New(e.DB, nil, nil, nil)).Work(ctx, job(jobs.RecheckTrackingDomainsArgs{})))

	kinds := e.queued(t)
	assert.Len(t, kinds, due)
	for _, k := range kinds {
		assert.Equal(t, "tracking_domain_verify", k)
	}
}
