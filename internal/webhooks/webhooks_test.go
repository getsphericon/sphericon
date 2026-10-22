package webhooks_test

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/webhook"
	"github.com/getsphericon/sphericon/internal/webhooks"
)

type license bool

func (l license) Licensed() bool { return bool(l) }

func module(env *testhelper.TestEnv, lic webhooks.Licensing) *webhooks.Module {
	return webhooks.New(env.Bus, env.Cipher, lic, webhook.NewClient(2*time.Second), nil)
}

// moduleAllowing is the module with a client the Instance's allowlist lets reach rcv.
func moduleAllowing(t *testing.T, env *testhelper.TestEnv, rcv *receiver) *webhooks.Module {
	t.Helper()
	u, err := url.Parse(rcv.srv.URL)
	require.NoError(t, err)
	allow, err := webhook.ParseAllowlist(u.Host)
	require.NoError(t, err)
	return webhooks.New(env.Bus, env.Cipher, nil, webhook.NewClientAllowing(2*time.Second, allow), nil)
}

// Fixtures (workspace 1): endpoint 100 (codebasics, filtered, secret whsec_Mf...)
// and 101 (disabled). Workspace 2 owns endpoint 900.
func TestCreateReturnsTheEndpointWithItsDecryptedStandardWebhooksSecret(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	ctx := context.Background()

	got, err := module(env, license(true)).Create(ctx, acme, webhooks.CreateInput{
		URL: "https://example.com/hook", EventTypes: []string{"contact.created"},
	})
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/hook", got.URL)
	assert.Nil(t, got.DisabledAt)
	require.True(t, strings.HasPrefix(got.Secret, "whsec_"))
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(got.Secret, "whsec_"))
	require.NoError(t, err)
	assert.Len(t, raw, 24)

	stored, err := acme.WebhookEndpoint().Get(ctx, got.ID)
	require.NoError(t, err)
	assert.NotContains(t, string(stored.SecretEncrypted), got.Secret, "stored sealed")

	again, err := module(env, license(true)).Get(ctx, acme, got.ID)
	require.NoError(t, err)
	assert.Equal(t, got.Secret, again.Secret)
}

func TestCreateHonoursEnabledFalse(t *testing.T) {
	env := testhelper.Setup(t)
	off := false
	got, err := module(env, nil).Create(context.Background(), env.DB.Scoped(fixtures.AcmeID),
		webhooks.CreateInput{URL: "https://example.com/off", Enabled: &off})
	require.NoError(t, err)
	assert.NotNil(t, got.DisabledAt)
	require.NotNil(t, got.DisabledReason)
	assert.Equal(t, webhookendpoint.DisabledReasonOwner, *got.DisabledReason)
}

func TestDisablingAnEndpointThatIsOffKeepsItsMomentAndReason(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	m := module(env, nil)
	before, err := m.Get(context.Background(), acme, fixtures.WebhookCodebasicsGoneID)
	require.NoError(t, err)

	off := false
	got, err := m.Update(context.Background(), acme, fixtures.WebhookCodebasicsGoneID, webhooks.UpdateInput{Enabled: &off})

	require.NoError(t, err)
	assert.Equal(t, before.DisabledAt, got.DisabledAt)
	assert.Equal(t, before.DisabledReason, got.DisabledReason)
}

func TestInvalidURLIsRefusedOnCreateAndUpdate(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	m := module(env, nil)
	ctx := context.Background()

	for _, url := range []string{"ftp://example.com/hook", "example.com/hook", "https://", "/relative", "", "http://%zz", "not-a-url"} {
		_, err := m.Create(ctx, acme, webhooks.CreateInput{URL: url})
		require.ErrorIs(t, err, webhooks.ErrInvalidURL, url)

		bad := url
		_, err = m.Update(ctx, acme, fixtures.WebhookCodebasicsID, webhooks.UpdateInput{URL: &bad})
		require.ErrorIs(t, err, webhooks.ErrInvalidURL, url)
	}
	ok := "http://example.com:8080/x"
	got, err := m.Update(ctx, acme, fixtures.WebhookCodebasicsID, webhooks.UpdateInput{URL: &ok})
	require.NoError(t, err)
	assert.Equal(t, ok, got.URL)
}

func TestUpdateChangesOnlyTheGivenFieldsAndKeepsTheSecret(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	m := module(env, nil)
	ctx := context.Background()

	before, err := m.Get(ctx, acme, fixtures.WebhookCodebasicsID)
	require.NoError(t, err)
	assert.Equal(t, "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw", before.Secret)

	off := false
	got, err := m.Update(ctx, acme, fixtures.WebhookCodebasicsID, webhooks.UpdateInput{Enabled: &off})
	require.NoError(t, err)
	assert.NotNil(t, got.DisabledAt)
	assert.Equal(t, before.URL, got.URL)
	assert.Equal(t, before.EventTypes, got.EventTypes, "nil event types leave the filter alone")
	assert.Equal(t, before.Secret, got.Secret)

	got, err = m.Update(ctx, acme, fixtures.WebhookCodebasicsID, webhooks.UpdateInput{EventTypes: []string{"email.opened"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"email.opened"}, got.EventTypes)
}

func TestAuditEntryNeedsALicenseOnCreateAndUpdate(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	ctx := context.Background()
	audit := []string{events.NameAuditEntry}

	for name, lic := range map[string]webhooks.Licensing{"nil": nil, "unlicensed": license(false)} {
		m := module(env, lic)
		_, err := m.Create(ctx, acme, webhooks.CreateInput{URL: "https://example.com/a", EventTypes: audit})
		require.ErrorIs(t, err, webhooks.ErrAuditNeedsLicense, name)
		_, err = m.Update(ctx, acme, fixtures.WebhookCodebasicsID, webhooks.UpdateInput{EventTypes: audit})
		require.ErrorIs(t, err, webhooks.ErrAuditNeedsLicense, name)
	}

	m := module(env, license(true))
	_, err := m.Create(ctx, acme, webhooks.CreateInput{URL: "https://example.com/a", EventTypes: audit})
	require.NoError(t, err)
	_, err = m.Update(ctx, acme, fixtures.WebhookCodebasicsID, webhooks.UpdateInput{EventTypes: audit})
	require.NoError(t, err)
}

func TestListPagesTheWorkspaceAscendingByID(t *testing.T) {
	env := testhelper.Setup(t)
	m := module(env, nil)
	ctx := context.Background()

	page, err := m.List(ctx, env.DB.Scoped(fixtures.AcmeID), pagination.Params{Page: 1, PageSize: 1})
	require.NoError(t, err)
	total, err := env.DB.Scoped(fixtures.AcmeID).WebhookEndpoint().Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, total, page.TotalItems)
	assert.Equal(t, total, page.TotalPages)
	require.Len(t, page.Items, 1)
	assert.Equal(t, int64(fixtures.WebhookCodebasicsID), page.Items[0].ID)
	assert.Equal(t, "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw", page.Items[0].Secret)

	second, err := m.List(ctx, env.DB.Scoped(fixtures.AcmeID), pagination.Params{Page: 2, PageSize: 1})
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.Equal(t, int64(fixtures.WebhookCodebasicsDisabledID), second.Items[0].ID)
}

func TestDeleteAndWorkspaceIsolation(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	m := module(env, nil)
	ctx := context.Background()

	_, err := m.Get(ctx, acme, fixtures.WebhookGlobexID)
	assert.True(t, ent.IsNotFound(err))
	_, err = m.Update(ctx, acme, fixtures.WebhookGlobexID, webhooks.UpdateInput{})
	assert.True(t, ent.IsNotFound(err))
	assert.True(t, ent.IsNotFound(m.Delete(ctx, acme, fixtures.WebhookGlobexID)))

	require.NoError(t, m.Delete(ctx, acme, fixtures.WebhookCodebasicsDisabledID))
	_, err = m.Get(ctx, acme, fixtures.WebhookCodebasicsDisabledID)
	assert.True(t, ent.IsNotFound(err))
}

// receiver is a webhook receiver that verifies the signature of every request with the
// secret it was built for and answers with a fixed status.
type receiver struct {
	srv *httptest.Server
	mu  sync.Mutex
	got []delivery
}

type delivery struct {
	body     []byte
	event    string
	verified error
}

func newReceiver(t *testing.T, secret string, status int) *receiver {
	t.Helper()
	r := &receiver{}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		wh, _ := standardwebhooks.NewWebhook(secret)
		r.mu.Lock()
		r.got = append(r.got, delivery{body: body, event: req.Header.Get(webhook.EventHeader), verified: wh.Verify(body, req.Header)})
		r.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *receiver) deliveries() []delivery {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]delivery(nil), r.got...)
}

func pointAt(t *testing.T, env *testhelper.TestEnv, id int64, target string) {
	t.Helper()
	_, err := env.DB.Scoped(fixtures.AcmeID).WebhookEndpoint().UpdateOneID(id).SetURL(target).Save(context.Background())
	require.NoError(t, err)
}

func TestRegenerateSecretSignsWithTheNewSecretOnly(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	ctx := context.Background()
	const oldSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

	got, err := module(env, nil).RegenerateSecret(ctx, acme, fixtures.WebhookCodebasicsID)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(got.Secret, "whsec_"))
	assert.NotEqual(t, oldSecret, got.Secret)

	again, err := module(env, nil).Get(ctx, acme, fixtures.WebhookCodebasicsID)
	require.NoError(t, err)
	assert.Equal(t, got.Secret, again.Secret)

	rcv := newReceiver(t, got.Secret, http.StatusOK)
	pointAt(t, env, fixtures.WebhookCodebasicsID, rcv.srv.URL)
	_, err = moduleAllowing(t, env, rcv).TestSend(ctx, acme, fixtures.WebhookCodebasicsID, "contact.created")
	require.NoError(t, err)
	oldRcv := newReceiver(t, oldSecret, http.StatusOK)
	pointAt(t, env, fixtures.WebhookCodebasicsID, oldRcv.srv.URL)
	_, err = moduleAllowing(t, env, oldRcv).TestSend(ctx, acme, fixtures.WebhookCodebasicsID, "contact.created")
	require.NoError(t, err)

	require.Len(t, rcv.deliveries(), 1)
	require.NoError(t, rcv.deliveries()[0].verified, "the new secret verifies")
	require.Len(t, oldRcv.deliveries(), 1)
	assert.Error(t, oldRcv.deliveries()[0].verified, "the old secret no longer verifies")
}

func TestRegenerateSecretOfAnotherWorkspaceIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := module(env, nil).RegenerateSecret(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.WebhookGlobexID)
	assert.True(t, ent.IsNotFound(err))
}

func TestTestSendDeliversASignedSampleOfTheChosenTypeAndReportsTheStatus(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	rcv := newReceiver(t, "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw", http.StatusAccepted)
	pointAt(t, env, fixtures.WebhookCodebasicsID, rcv.srv.URL)

	res, err := moduleAllowing(t, env, rcv).TestSend(context.Background(), acme, fixtures.WebhookCodebasicsID, "email.opened")
	require.NoError(t, err)
	assert.True(t, res.Delivered)
	assert.Equal(t, http.StatusAccepted, res.Status)
	assert.Empty(t, res.Kind)

	got := rcv.deliveries()
	require.Len(t, got, 1)
	require.NoError(t, got[0].verified)
	assert.Equal(t, "email.opened", got[0].event)
	assert.Contains(t, string(got[0].body), `"type":"email.opened"`)
}

func TestTestSendReportsTheErrorKindAndLeavesTheFailureStateAlone(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	ctx := context.Background()
	rcv := newReceiver(t, "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw", http.StatusGone)
	pointAt(t, env, fixtures.WebhookCodebasicsFailingID, rcv.srv.URL)
	before, err := acme.WebhookEndpoint().Get(ctx, fixtures.WebhookCodebasicsFailingID)
	require.NoError(t, err)
	require.NotNil(t, before.ConsecutiveFailures)

	res, err := moduleAllowing(t, env, rcv).TestSend(ctx, acme, fixtures.WebhookCodebasicsFailingID, "email.opened")
	require.NoError(t, err)
	assert.False(t, res.Delivered)
	assert.Equal(t, webhook.KindHTTPStatus, res.Kind)
	assert.Equal(t, http.StatusGone, res.Status)

	// Without the allowlist the loopback receiver is a blocked address.
	res, err = module(env, nil).TestSend(ctx, acme, fixtures.WebhookCodebasicsFailingID, "email.opened")
	require.NoError(t, err)
	assert.Equal(t, webhook.KindBlockedAddress, res.Kind)
	assert.Zero(t, res.Status)

	after, err := acme.WebhookEndpoint().Get(ctx, fixtures.WebhookCodebasicsFailingID)
	require.NoError(t, err)
	assert.Equal(t, before.ConsecutiveFailures, after.ConsecutiveFailures)
	assert.Equal(t, before.LastErrorKind, after.LastErrorKind)
	assert.Equal(t, before.LastErrorStatus, after.LastErrorStatus)
	assert.True(t, before.LastFailureAt.Equal(*after.LastFailureAt))
	assert.Nil(t, after.DisabledAt)
}

func TestTestSendWorksOnADisabledEndpointAndRefusesABlankType(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	ctx := context.Background()
	rcv := newReceiver(t, "whsec_Y29kZWJhc2ljc19kaXNhYmxlZF9zZWNyZXQ=", http.StatusOK)
	pointAt(t, env, fixtures.WebhookCodebasicsDisabledID, rcv.srv.URL)
	m := moduleAllowing(t, env, rcv)

	res, err := m.TestSend(ctx, acme, fixtures.WebhookCodebasicsDisabledID, "contact.created")
	require.NoError(t, err)
	assert.True(t, res.Delivered)

	_, err = m.TestSend(ctx, acme, fixtures.WebhookCodebasicsDisabledID, " ")
	require.ErrorIs(t, err, webhooks.ErrInvalidEventType)
	_, err = m.TestSend(ctx, acme, fixtures.WebhookGlobexID, "contact.created")
	assert.True(t, ent.IsNotFound(err))
}

func TestTestSendIsAuditedAsAWebhookEndpointTest(t *testing.T) {
	env := testhelper.Setup(t)
	acting := env.Bus.Act(env.DB.Scoped(fixtures.AcmeID), events.Actor{Kind: events.ActorUser, ID: "7", Name: "John"})
	rcv := newReceiver(t, "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw", http.StatusOK)
	pointAt(t, env, fixtures.WebhookCodebasicsID, rcv.srv.URL)
	before := len(env.OutboxEvents(t, events.NameAuditEntry))

	_, err := moduleAllowing(t, env, rcv).TestSend(context.Background(), acting, fixtures.WebhookCodebasicsID, "email.opened")
	require.NoError(t, err)

	all := env.OutboxEvents(t, events.NameAuditEntry)
	require.Len(t, all, before+1)
	e := all[len(all)-1].(*events.AuditEntry)
	assert.Equal(t, events.ActionWebhookEndpointTest, e.Action)
	assert.Equal(t, "webhook_endpoint", e.TargetType)
	assert.Equal(t, "John", e.Actor.Name)
	assert.Equal(t, "email.opened", e.Diff["eventType"])
}

// Enabling an endpoint that turned itself off starts it fresh: the reason and the failure
// state are gone. Disabling it by hand drops the reason too, since the owner decided now.
func TestEnablingClearsTheFailureStateAndTheDisabledReason(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	m := module(env, nil)
	ctx := context.Background()
	on := true

	before, err := acme.WebhookEndpoint().Get(ctx, fixtures.WebhookCodebasicsGoneID)
	require.NoError(t, err)
	require.NotNil(t, before.DisabledReason)
	require.NotNil(t, before.ConsecutiveFailures)

	got, err := m.Update(ctx, acme, fixtures.WebhookCodebasicsGoneID, webhooks.UpdateInput{Enabled: &on})
	require.NoError(t, err)
	assert.Nil(t, got.DisabledAt)
	assert.Nil(t, got.DisabledReason)
	assert.Nil(t, got.ConsecutiveFailures)
	assert.Nil(t, got.FailingSince)
	assert.Nil(t, got.LastErrorKind)
	assert.Nil(t, got.LastErrorStatus)
	assert.Nil(t, got.LastFailureAt)
}

// owners records the Owners' notices the module asks for.
type owners struct {
	mu   sync.Mutex
	sent []notice
}

type notice struct {
	workspaceID   int64
	subject, body string
}

func (o *owners) notify(_ context.Context, workspaceID int64, subject, body string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sent = append(o.sent, notice{workspaceID, subject, body})
	return nil
}

func (o *owners) notices() []notice {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]notice(nil), o.sent...)
}

// recording is the module over a stub Owners' notifier, with the clock the test injects.
type recording struct {
	t      *testing.T
	env    *testhelper.TestEnv
	mod    *webhooks.Module
	owners *owners
	now    time.Time
}

func newRecording(t *testing.T) *recording {
	t.Helper()
	env := testhelper.Setup(t)
	o := &owners{}
	return &recording{
		t: t, env: env, owners: o,
		mod: webhooks.New(env.Bus, env.Cipher, nil, webhook.NewClient(2*time.Second), o.notify),
		now: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC),
	}
}

func (r *recording) scope() *ent.Scoped { return r.env.DB.Scoped(fixtures.AcmeID) }

func (r *recording) endpoint() *ent.WebhookEndpoint {
	r.t.Helper()
	e, err := r.scope().WebhookEndpoint().Get(context.Background(), fixtures.WebhookCodebasicsID)
	require.NoError(r.t, err)
	return e
}

// midRun puts the endpoint mid-run: failures already in a row, the first of them failingFor ago.
func (r *recording) midRun(failures int, failingFor time.Duration) {
	r.t.Helper()
	_, err := r.scope().WebhookEndpoint().UpdateOneID(fixtures.WebhookCodebasicsID).
		SetConsecutiveFailures(failures).SetFailingSince(r.now.Add(-failingFor)).Save(context.Background())
	require.NoError(r.t, err)
}

// record reports one delivery outcome for the endpoint as it is stored now.
func (r *recording) record(sendErr error) {
	r.t.Helper()
	require.NoError(r.t, r.mod.RecordDelivery(context.Background(), r.scope(), r.endpoint(), sendErr, r.now))
}

func status(code int) error {
	return &webhook.DeliveryError{Kind: webhook.KindHTTPStatus, Status: code}
}

// A failed delivery is remembered with a closed error kind, the run keeps its start, and a
// success clears it all. A failure alone never disables.
func TestRecordDeliveryRemembersFailuresAndASuccessClearsThem(t *testing.T) {
	r := newRecording(t)

	r.record(nil)
	healthy := r.endpoint()
	assert.Nil(t, healthy.ConsecutiveFailures)

	r.record(status(http.StatusServiceUnavailable))
	e := r.endpoint()
	require.NotNil(t, e.ConsecutiveFailures)
	assert.Equal(t, 1, *e.ConsecutiveFailures)
	assert.True(t, r.now.Equal(*e.FailingSince))
	assert.True(t, r.now.Equal(*e.LastFailureAt))
	assert.Equal(t, webhookendpoint.LastErrorKindHTTPStatus, *e.LastErrorKind)
	assert.Equal(t, http.StatusServiceUnavailable, *e.LastErrorStatus)
	assert.Nil(t, e.DisabledAt)

	began := r.now
	r.now = r.now.Add(90 * time.Minute)
	r.record(&webhook.DeliveryError{Kind: webhook.KindBlockedAddress})
	e = r.endpoint()
	assert.Equal(t, 2, *e.ConsecutiveFailures)
	assert.True(t, began.Equal(*e.FailingSince))
	assert.True(t, r.now.Equal(*e.LastFailureAt))
	assert.Equal(t, webhookendpoint.LastErrorKindBlockedAddress, *e.LastErrorKind)
	assert.Nil(t, e.LastErrorStatus)

	r.record(nil)
	e = r.endpoint()
	assert.Nil(t, e.ConsecutiveFailures)
	assert.Nil(t, e.FailingSince)
	assert.Nil(t, e.LastErrorKind)
	assert.Nil(t, e.LastErrorStatus)
	assert.Nil(t, e.LastFailureAt)
}

// A 429 is the receiver asking us to slow down: the counters stay as they are. So does an
// error that is not a delivery outcome (an unusable secret).
func TestRecordDeliveryLeavesTheCountersAloneOnA429OrANonDeliveryError(t *testing.T) {
	r := newRecording(t)
	r.midRun(3, time.Hour)

	r.record(status(http.StatusTooManyRequests))
	r.record(errors.New("unusable secret"))

	e := r.endpoint()
	assert.Equal(t, 3, *e.ConsecutiveFailures)
	assert.Nil(t, e.DisabledAt)
}

// A 410 retires the endpoint on that delivery, whatever the run so far: with the reason, an
// Audit entry by the system actor (the counter writes are not audited), and one notice to the
// owners that names the endpoint's origin only.
func TestRecordDeliveryDisablesOnGoneAndTellsTheOwnersOnce(t *testing.T) {
	r := newRecording(t)
	_, err := r.scope().WebhookEndpoint().UpdateOneID(fixtures.WebhookCodebasicsID).
		SetURL("https://hooks.example.com/path/hidden?token=abc").Save(context.Background())
	require.NoError(t, err)

	r.record(status(http.StatusGone))

	e := r.endpoint()
	assert.NotNil(t, e.DisabledAt)
	require.NotNil(t, e.DisabledReason)
	assert.Equal(t, webhookendpoint.DisabledReasonGone, *e.DisabledReason)

	sent := r.owners.notices()
	require.Len(t, sent, 1)
	assert.EqualValues(t, fixtures.AcmeID, sent[0].workspaceID)
	assert.Contains(t, sent[0].subject, "webhook")
	assert.Contains(t, sent[0].body, "410")
	assert.Contains(t, sent[0].body, "https://hooks.example.com")
	assert.NotContains(t, sent[0].body, "hidden")

	entries := lo.FilterMap(r.env.OutboxEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) (*events.AuditEntry, bool) {
		a := ev.(*events.AuditEntry)
		return a, a.Action == "webhook_endpoint.update"
	})
	require.Len(t, entries, 1)
	assert.Equal(t, events.ActorSystem, entries[0].Actor.Kind)
	assert.Contains(t, entries[0].Diff, "disabled_at")

	// A second failure that lost the race finds it off and tells nobody.
	r.record(status(http.StatusGone))
	assert.Len(t, r.owners.notices(), 1)
}

// A run of failures disables only when it has both reached the count and lasted the window.
func TestRecordDeliveryDisablesOnlyWhenTheRunReachedTheCountAndTheWindow(t *testing.T) {
	r := newRecording(t)

	r.midRun(webhooks.DisableFailures-1, webhooks.DisableWindow-time.Minute)
	r.record(status(http.StatusServiceUnavailable))
	e := r.endpoint()
	assert.Equal(t, webhooks.DisableFailures, *e.ConsecutiveFailures)
	assert.Nil(t, e.DisabledAt, "the count inside the window (a restart mid-burst)")

	r.midRun(webhooks.DisableFailures-2, 2*webhooks.DisableWindow)
	r.record(status(http.StatusServiceUnavailable))
	assert.Nil(t, r.endpoint().DisabledAt, "the window without the count")
	assert.Empty(t, r.owners.notices())

	r.midRun(webhooks.DisableFailures-1, webhooks.DisableWindow)
	r.record(status(http.StatusServiceUnavailable))
	e = r.endpoint()
	assert.NotNil(t, e.DisabledAt)
	require.NotNil(t, e.DisabledReason)
	assert.Equal(t, webhookendpoint.DisabledReasonPersistentFailure, *e.DisabledReason)
	assert.Len(t, r.owners.notices(), 1)
}

// Without a notifier the endpoint still disables.
func TestRecordDeliveryDisablesWithoutANotifier(t *testing.T) {
	env := testhelper.Setup(t)
	mod := webhooks.New(env.Bus, env.Cipher, nil, webhook.NewClient(2*time.Second), nil)
	acme := env.DB.Scoped(fixtures.AcmeID)
	e, err := acme.WebhookEndpoint().Get(context.Background(), fixtures.WebhookCodebasicsID)
	require.NoError(t, err)

	err = mod.RecordDelivery(context.Background(), acme, e, status(http.StatusGone), time.Now())
	require.NoError(t, err)

	after, err := acme.WebhookEndpoint().Get(context.Background(), fixtures.WebhookCodebasicsID)
	require.NoError(t, err)
	assert.NotNil(t, after.DisabledAt)
}
