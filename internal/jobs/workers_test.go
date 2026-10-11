package jobs_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mokevnin/sphericon/config"
	"github.com/mokevnin/sphericon/ent"
	"github.com/mokevnin/sphericon/ent/automation"
	"github.com/mokevnin/sphericon/ent/automationrun"
	"github.com/mokevnin/sphericon/ent/broadcast"
	"github.com/mokevnin/sphericon/ent/broadcastrecipient"
	"github.com/mokevnin/sphericon/internal/dnstest"
	"github.com/mokevnin/sphericon/internal/fixtures"
	"github.com/mokevnin/sphericon/internal/jobs"
	"github.com/mokevnin/sphericon/internal/messaging/registry"
	"github.com/mokevnin/sphericon/internal/secrets"
	"github.com/mokevnin/sphericon/internal/suspension"
	"github.com/mokevnin/sphericon/internal/testhelper"
)

// riverEnv is a real river client over the test database. The workers read and
// write through the test's txdb-bound ent client (rolled back with the test), while
// the queue itself is real: river inserts commit on their own pool, so the test
// clears river_job before and after.
type riverEnv struct {
	*testhelper.TestEnv
	pool   *pgxpool.Pool
	client *jobs.Client
	cipher *secrets.Cipher
	cfg    *config.Config
	sender *fakeSender
}

func newRiverEnv(t *testing.T) *riverEnv {
	t.Helper()
	env := testhelper.Setup(t)
	ctx := context.Background()

	cfg, err := config.Load("test")
	require.NoError(t, err)
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, jobs.Migrate(ctx, pool))

	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	fs := &fakeSender{}
	mod := newMod(env, resolvingTo(fs))
	client, err := jobs.NewClient(pool, env.DB, env.SQLDB, mod, cipher, env.SystemMail, nil, registry.Default(), cfg.AppURL, jobs.Retention{OutboxFloor: cfg.OutboxFloor, Events: cfg.EventsRetention})
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
	_, err := e.client.River().JobDeleteMany(context.Background(), river.NewJobDeleteManyParams().Queues(river.QueueDefault, jobs.QueueBroadcasts, jobs.QueueWebhooks))
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

	require.NoError(t, e.client.EnqueueBroadcast(ctx, fixtures.BroadcastDraftID, nil))
	later := time.Now().Add(time.Hour)
	require.NoError(t, e.client.EnqueueBroadcast(ctx, fixtures.BroadcastDraftID, &later))
	require.NoError(t, e.client.EnqueueWelcome(ctx, "new@example.com", "New"))
	require.NoError(t, e.client.EnqueuePasswordReset(ctx, "a@example.com", "tok", true))
	require.NoError(t, e.client.EnqueueEmailVerification(ctx, "a@example.com", "tok"))
	require.NoError(t, e.client.EnqueueEmailChangeConfirm(ctx, "a@example.com", "tok"))
	require.NoError(t, e.client.EnqueueMemberInvite(ctx, "a@example.com", "https://x/invite", "Acme", "Jane"))
	require.NoError(t, e.client.EnqueueSendingDomainVerify(ctx, fixtures.SendingDomainVerifiedID))
	require.NoError(t, e.client.EnqueueSecondFactorRequired(ctx, fixtures.UmbrellaID))
	require.NoError(t, e.client.OnEvent(ctx, fixtures.AcmeID, fixtures.ContactAliceID, "contact.created"))

	assert.Equal(t, []string{
		"send_broadcast", "send_broadcast", "send_welcome",
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

// recordingDoer captures webhook deliveries and answers with a fixed status.
type recordingDoer struct {
	mu     sync.Mutex
	status int
	err    error
	urls   []string
	bodies []string
}

func (d *recordingDoer) Do(req *http.Request) (*http.Response, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return nil, d.err
	}
	b, _ := io.ReadAll(req.Body)
	d.urls = append(d.urls, req.URL.String())
	d.bodies = append(d.bodies, string(b))
	return &http.Response{StatusCode: d.status, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
}

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

	doer := &recordingDoer{status: 200}
	w := jobs.NewDeliverWebhookWorker(env.DB, cipher, doer)
	require.NoError(t, w.Work(ctx, job(args(fixtures.WebhookCodebasicsID))))
	assert.Equal(t, []string{fixtures.WebhookCodebasicsURL}, doer.urls)
	assert.JSONEq(t, `{"a":1}`, doer.bodies[0])

	// Deleted or disabled since enqueue: dropped without a request.
	require.NoError(t, w.Work(ctx, job(args(424242))))
	require.NoError(t, w.Work(ctx, job(args(101))))
	assert.Len(t, doer.urls, 1)

	// A non-2xx answer and a transport failure both surface so river retries.
	require.Error(t, jobs.NewDeliverWebhookWorker(env.DB, cipher, &recordingDoer{status: 500}).Work(ctx, job(args(fixtures.WebhookCodebasicsID))))
	require.Error(t, jobs.NewDeliverWebhookWorker(env.DB, cipher, &recordingDoer{err: errors.New("dial")}).Work(ctx, job(args(fixtures.WebhookCodebasicsID))))

}

func TestVerifySendingDomainWorker(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	gone := dnstest.Resolver(t, nil).LookupTXT

	// Verified domain whose DNS vanished: flips, owner is emailed.
	w := jobs.NewVerifySendingDomainWorker(env.DB, gone, env.SystemMail)
	require.NoError(t, w.Work(ctx, job(jobs.VerifySendingDomainArgs{SendingDomainID: fixtures.SendingDomainVerifiedID})))
	assert.Len(t, env.SystemMail.Messages(), 1)

	// A failing alert never fails the verification.
	env.SystemMail.SetErr(errors.New("smtp down"))
	require.NoError(t, jobs.NewVerifySendingDomainWorker(env.DB, gone, env.SystemMail).
		Work(ctx, job(jobs.VerifySendingDomainArgs{SendingDomainID: fixtures.SendingDomainVerifiedID})))

	// Unknown domain id is an error.
	require.Error(t, w.Work(ctx, job(jobs.VerifySendingDomainArgs{SendingDomainID: 424242})))
}

func TestRecheckSendingDomainsWorkerFansOut(t *testing.T) {
	e := newRiverEnv(t)
	total, err := e.DB.SendingDomain.Query().Count(context.Background())
	require.NoError(t, err)

	require.NoError(t, jobs.NewRecheckSendingDomainsWorker(e.DB).Work(e.workCtx(), job(jobs.RecheckSendingDomainsArgs{})))

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
	_, err = e.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Flow").SetTriggerEvent("worker.test").SetStatus(automation.StatusActive).
		SetDefinition("[" + emailStep + `,{"type":"wait","seconds":3600},` + emailStep + "]").Save(ctx)
	require.NoError(t, err)

	require.NoError(t, jobs.NewEvaluateTriggerWorker(e.DB).Work(ctx, job(jobs.EvaluateTriggerArgs{
		WorkspaceID: fixtures.AcmeID, ContactID: c.ID, Action: "worker.test",
	})))
	require.Equal(t, []string{"automation_run_step"}, e.queued(t))
	run := e.DB.AutomationRun.Query().Where(automationrun.ContactID(c.ID)).OnlyX(ctx)

	// Step 1 sends and schedules the next step; the wait step schedules a delayed
	// one; the last step finishes the run without queueing more.
	w := jobs.NewRunStepWorker(e.DB, newMod(e.TestEnv, resolvingTo(e.sender)))
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
	require.NoError(t, w.Work(ctx, job(jobs.SendBroadcastArgs{BroadcastID: fixtures.BroadcastDraftID})))
	require.Empty(t, e.queued(t))

	// Fan-out: one recipient job per eligible contact.
	e.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetStatus(broadcast.StatusSending).ExecX(ctx)
	require.NoError(t, w.Work(ctx, job(jobs.SendBroadcastArgs{BroadcastID: fixtures.BroadcastDraftID})))
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
	require.NoError(t, w.Work(ctx, job(jobs.SendBroadcastArgs{BroadcastID: fixtures.BroadcastEmptyAudienceID})))
	assert.Len(t, e.queued(t), before)

	// A suspended workspace holds: the job snoozes rather than fails.
	_, err := suspension.SuspendWorkspace(ctx, e.Bus, fixtures.AcmeID, suspension.System, "complaints")
	require.NoError(t, err)
	e.DB.Broadcast.UpdateOneID(fixtures.BroadcastProSegmentID).SetStatus(broadcast.StatusSending).ExecX(ctx)
	var snooze *river.JobSnoozeError
	require.ErrorAs(t, w.Work(ctx, job(jobs.SendBroadcastArgs{BroadcastID: fixtures.BroadcastProSegmentID})), &snooze)

	// A resolver failure is a plain error: river retries it.
	hold := jobs.NewSendBroadcastWorker(e.DB, newMod(e.TestEnv, failingResolver(errors.New("no integration"))))
	require.Error(t, hold.Work(ctx, job(jobs.SendBroadcastArgs{BroadcastID: fixtures.BroadcastProSegmentID})))
}

func TestSendRecipientWorkerRecordsTerminalFailure(t *testing.T) {
	e := newRiverEnv(t)
	ctx := e.workCtx()
	mod := newMod(e.TestEnv, resolvingTo(e.sender))
	ids, err := jobs.PlanBroadcast(ctx, e.DB, mod, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	require.NotEmpty(t, ids)

	failing := jobs.NewSendRecipientWorker(e.DB, newMod(e.TestEnv, resolvingTo(erroringSender{})))
	// Not the last attempt: the error surfaces for retry, row stays pending.
	err = failing.Work(ctx, job(jobs.SendRecipientArgs{RecipientID: ids[0], BroadcastID: fixtures.BroadcastDraftID}))
	require.Error(t, err)
	assert.Equal(t, broadcastrecipient.StatusPending, e.DB.BroadcastRecipient.GetX(ctx, ids[0]).Status)

	// Last attempt: the row is marked failed so the broadcast can finalize.
	last := job(jobs.SendRecipientArgs{RecipientID: ids[0], BroadcastID: fixtures.BroadcastDraftID})
	last.Attempt = last.MaxAttempts
	require.Error(t, failing.Work(ctx, last))
	assert.Equal(t, broadcastrecipient.StatusFailed, e.DB.BroadcastRecipient.GetX(ctx, ids[0]).Status)
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

func TestHeldErrorMessage(t *testing.T) {
	assert.Equal(t, "send held: no_integration", (&jobs.HeldError{Reason: "no_integration"}).Error())
}
