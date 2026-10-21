package server_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-faster/jx"
	"github.com/samber/lo"
	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/suppression"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/testhelper/metricstest"
)

// The hook over HTTP, on the committed Integrations of Acme that read their callbacks with
// the generic Preset (ADR 0037). Every case posts a signed request to a hook URL and asserts
// what it left behind: the response, the outbox, Suppression.

// hookClock is the instant the hook believes it is, and the one requests are signed at.
var hookClock = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// The secrets of the two generic Integrations in fixtures/integrations.yml: whsec_ and the
// base64 of a plain string, which is how the fixture spells them.
var (
	secretA = newSecret("secret-A0123456789")
	secretB = newSecret("secret-B0123456789")
)

func newSecret(seed string) string {
	return "whsec_" + base64.StdEncoding.EncodeToString([]byte("sphericon-test-"+seed))
}

func setupHooks(t *testing.T) *testhelper.TestEnv {
	t.Helper()
	return testhelper.Setup(t, testhelper.WithClock(func() time.Time { return hookClock }))
}

// signedPost posts body to the hook of key, signed with secret as Standard Webhooks does.
func signedPost(t *testing.T, env *testhelper.TestEnv, key, secret, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	wh, err := standardwebhooks.NewWebhook(secret)
	require.NoError(t, err)
	sig, err := wh.Sign(id, hookClock, []byte(body))
	require.NoError(t, err)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/hooks/"+key, strings.NewReader(body))
	req.Header.Set("webhook-id", id)
	req.Header.Set("webhook-timestamp", strconv.FormatInt(hookClock.Unix(), 10))
	req.Header.Set("webhook-signature", sig)
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

func failureBody(id, destination string, permanent bool) string {
	return `{"id":"` + id + `","destination":"` + destination + `","kind":"bounce","permanent":` + strconv.FormatBool(permanent) + `,"sending_domain":"Codebasics.dev"}`
}

// applySuppression plays the suppression consumer over everything published so far.
func applySuppression(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	for _, envelope := range env.OutboxEnvelopes(t, events.NameEmailBounced, events.NameEmailComplained) {
		require.NoError(t, events.Suppress(t.Context(), env.DB, envelope))
	}
}

func suppressed(t *testing.T, env *testhelper.TestEnv, email string) bool {
	t.Helper()
	ok, err := env.DB.Suppression.Query().
		Where(suppression.WorkspaceID(fixtures.AcmeID), suppression.Destination(email), suppression.LiftedAtIsNil()).
		Exist(t.Context())
	require.NoError(t, err)
	return ok
}

func TestHookOfAnUnknownKeyIsNotFound(t *testing.T) {
	env := setupHooks(t)

	rec := signedPost(t, env, "omik_does_not_exist", secretA, "msg_1", failureBody("evt_1", "a@example.com", true))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, outboxEvents(t, env, "a@example.com"))
}

// A Workspace in its deletion window takes no new data (ADR 0043): its hook answers 404 and
// publishes nothing, and answers again once the Workspace is restored.
func TestHookOfADeletedWorkspaceIsNotFound(t *testing.T) {
	env := setupHooks(t)
	key := fixtures.IntegrationAcmeGenericAIngestKey
	body := failureBody("evt_1", fixtures.ContactAliceEmail, true)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(hookClock).ExecX(t.Context())

	assert.Equal(t, http.StatusNotFound, signedPost(t, env, key, secretA, "msg_1", body).Code)
	assert.Empty(t, outboxEvents(t, env, fixtures.ContactAliceEmail))

	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).ClearDiscardedAt().ExecX(t.Context())
	assert.Equal(t, http.StatusOK, signedPost(t, env, key, secretA, "msg_1", body).Code)
}

// An Integration accepts only its own secret: a callback signed for one Integration of the
// Workspace is refused at the other's URL, and changes nothing.
func TestTwoIntegrationsOfAWorkspaceEachAcceptOnlyTheirOwnSecret(t *testing.T) {
	env := setupHooks(t)
	body := failureBody("evt_1", strings.ToUpper(fixtures.ContactAliceEmail), true)

	assert.Equal(t, http.StatusForbidden, signedPost(t, env, fixtures.IntegrationAcmeGenericBIngestKey, secretA, "msg_1", body).Code)
	assert.Equal(t, http.StatusForbidden, signedPost(t, env, fixtures.IntegrationAcmeGenericAIngestKey, secretB, "msg_1", body).Code)
	assert.Empty(t, outboxEvents(t, env, fixtures.ContactAliceEmail))

	require.Equal(t, http.StatusOK, signedPost(t, env, fixtures.IntegrationAcmeGenericAIngestKey, secretA, "msg_1", body).Code)
	require.Equal(t, http.StatusOK, signedPost(t, env, fixtures.IntegrationAcmeGenericBIngestKey, secretB, "msg_2", body).Code)
	got := outboxEvents(t, env, fixtures.ContactAliceEmail)
	require.Len(t, got, 2)
	for _, f := range got {
		assert.Equal(t, events.NameEmailBounced, f.Action)
		assert.Equal(t, events.BounceKindPermanent, f.BounceKind)
		assert.EqualValues(t, fixtures.ContactAliceID, f.ContactID)
		assert.Equal(t, "generic", f.Provider, "the Preset's name is the provider")
		assert.Equal(t, "codebasics.dev", f.SendingDomain)
	}
	assert.Equal(t, "evt_1/"+fixtures.ContactAliceEmail, got[0].DedupID, "the provider's event id plus the Destination")
}

func TestHookRefusesAnUnsignedOrUnreadableRequest(t *testing.T) {
	env := setupHooks(t)
	key := fixtures.IntegrationAcmeGenericAIngestKey

	unsigned := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/hooks/"+key, strings.NewReader(failureBody("evt_1", "a@example.com", true)))
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, unsigned)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// Correctly signed, but not JSON: the provider's to fix, not retried.
	assert.Equal(t, http.StatusBadRequest, signedPost(t, env, key, secretA, "msg_1", "not json").Code)

	assert.Empty(t, outboxEvents(t, env, "a@example.com"))
}

// A valid request with nothing the Preset handles is accepted, so the provider does not
// disable the endpoint, and publishes nothing.
func TestHookAcceptsAValidRequestWithNothingToHandle(t *testing.T) {
	env := setupHooks(t)

	rec := signedPost(t, env, fixtures.IntegrationAcmeGenericAIngestKey, secretA, "msg_1", `[]`)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, env.OutboxEvents(t, events.NameEmailBounced, events.NameEmailComplained))
}

// A hard bounce suppresses; a soft one is still published (the deliverability rates read
// it) and never suppresses.
func TestHookSuppressesAHardBounceAndOnlyRecordsASoftOne(t *testing.T) {
	env := setupHooks(t)
	key := fixtures.IntegrationAcmeGenericAIngestKey

	require.Equal(t, http.StatusOK, signedPost(t, env, key, secretA, "msg_1", failureBody("evt_hard", "hard@example.com", true)).Code)
	require.Equal(t, http.StatusOK, signedPost(t, env, key, secretA, "msg_2", failureBody("evt_soft", "soft@example.com", false)).Code)

	soft := outboxEvents(t, env, "soft@example.com")
	require.Len(t, soft, 1)
	assert.Equal(t, events.BounceKindTransient, soft[0].BounceKind)

	applySuppression(t, env)
	assert.True(t, suppressed(t, env, "hard@example.com"))
	assert.False(t, suppressed(t, env, "soft@example.com"))
}

// A hard bounce for a destination whose Suppression was lifted suppresses it anew.
func TestHookSuppressesALiftedDestinationAgain(t *testing.T) {
	env := setupHooks(t)
	key := fixtures.IntegrationAcmeGenericAIngestKey

	require.Equal(t, http.StatusOK, signedPost(t, env, key, secretA, "msg_1", failureBody("evt_again", "lifted@codebasics.dev", true)).Code)
	applySuppression(t, env)

	active, err := env.DB.Suppression.Query().
		Where(suppression.WorkspaceID(fixtures.AcmeID), suppression.Destination("lifted@codebasics.dev"), suppression.LiftedAtIsNil()).
		Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, active, "a new active row beside the lifted one")
}

// The same callback delivered twice has the effect of once: one persisted event, one
// Suppression, one count.
func TestHookRedeliveryHasTheEffectOfOnce(t *testing.T) {
	env := setupHooks(t)
	metricstest.StartMetrics(t)
	key := fixtures.IntegrationAcmeGenericAIngestKey
	body := failureBody("evt_1", fixtures.ContactAliceEmail, true)

	require.Equal(t, http.StatusOK, signedPost(t, env, key, secretA, "msg_1", body).Code)
	require.Equal(t, http.StatusOK, signedPost(t, env, key, secretA, "msg_1", body).Code)
	for _, envelope := range env.OutboxEnvelopes(t, events.NameEmailBounced) {
		require.NoError(t, events.Persist(t.Context(), env.DB, envelope))
	}
	applySuppression(t, env)

	n, err := env.DB.Event.Query().Where(event.SourceID("evt_1/" + fixtures.ContactAliceEmail)).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.True(t, suppressed(t, env, fixtures.ContactAliceEmail))
	scrape := metricstest.ScrapeMetrics(t)
	assert.InDelta(t, 1, scrape.Value(t, "email_send_outcomes_total", map[string]string{"provider": "generic", "status": "bounce"}), 0)
}

func updateParams(t *testing.T, c *externalapi.Client, id int64, update externalapi.UpdateIntegrationInput) *externalapi.IntegrationResource {
	t.Helper()
	res, err := c.IntegrationsUpdate(t.Context(), &update, externalapi.IntegrationsUpdateParams{ID: externalapi.EntityId(id)})
	require.NoError(t, err)
	out, ok := res.(*externalapi.IntegrationResource)
	require.Truef(t, ok, "got %T", res)
	return out
}

func secretParam(secret string) externalapi.OptUpdateIntegrationInputParams {
	return externalapi.NewOptUpdateIntegrationInputParams(externalapi.UpdateIntegrationInputParams{
		"secret": jx.Raw(strconv.Quote(secret)),
	})
}

// Rotating a secret keeps the old value valid until it is retired, then refuses it; no read
// returns either value.
func TestRotatingASecretKeepsTheOldOneValidUntilItIsRetired(t *testing.T) {
	env := setupHooks(t)
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	id := int64(fixtures.IntegrationAcmeGenericAID)
	key := fixtures.IntegrationAcmeGenericAIngestKey
	rotated := newSecret("rotated-0123456789")

	read := updateParams(t, c, id, externalapi.UpdateIntegrationInput{Params: secretParam(rotated)})
	assert.Equal(t, []string{"secret"}, read.SecretsRetiring)
	shown := string(read.Params["secret"])
	assert.NotContains(t, shown, rotated)
	assert.NotContains(t, shown, secretA)

	assert.Equal(t, http.StatusOK, signedPost(t, env, key, rotated, "msg_1", failureBody("evt_new", "new@example.com", true)).Code)
	assert.Equal(t, http.StatusOK, signedPost(t, env, key, secretA, "msg_2", failureBody("evt_old", "old@example.com", true)).Code)

	retired := updateParams(t, c, id, externalapi.UpdateIntegrationInput{RetireSecrets: externalapi.NewOptBool(true)})
	assert.Empty(t, retired.SecretsRetiring)
	assert.Equal(t, http.StatusOK, signedPost(t, env, key, rotated, "msg_3", failureBody("evt_new2", "new2@example.com", true)).Code)
	assert.Equal(t, http.StatusForbidden, signedPost(t, env, key, secretA, "msg_4", failureBody("evt_old2", "old2@example.com", true)).Code)
	assert.Empty(t, outboxEvents(t, env, "old2@example.com"))
}

// Regenerating the key invalidates the old URL; the new one works, with the same secret.
func TestRegeneratingTheKeyInvalidatesTheOldURL(t *testing.T) {
	env := setupHooks(t)
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	old := fixtures.IntegrationAcmeGenericAIngestKey

	res, err := c.IntegrationsRegenerateKey(t.Context(), externalapi.IntegrationsRegenerateKeyParams{ID: externalapi.EntityId(fixtures.IntegrationAcmeGenericAID)})
	require.NoError(t, err)
	got, ok := res.(*externalapi.IntegrationResource)
	require.Truef(t, ok, "got %T", res)
	require.Contains(t, got.HookUrl, "/hooks/omik_", got.HookUrl)
	require.NotContains(t, got.HookUrl, old)
	newKey := got.HookUrl[strings.LastIndex(got.HookUrl, "/")+1:]

	body := failureBody("evt_1", "a@example.com", true)
	assert.Equal(t, http.StatusNotFound, signedPost(t, env, old, secretA, "msg_1", body).Code)
	assert.Equal(t, http.StatusOK, signedPost(t, env, newKey, secretA, "msg_1", body).Code)
}

// Params that violate the Preset's schema are refused, and an unknown Preset too.
func TestIntegrationParamsAreValidatedAgainstThePresetSchema(t *testing.T) {
	env := setupHooks(t)
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	id := externalapi.EntityId(fixtures.IntegrationAcmeGenericAID)

	for name, in := range map[string]externalapi.UpdateIntegrationInput{
		"a secret of the wrong type": {Params: externalapi.NewOptUpdateIntegrationInputParams(externalapi.UpdateIntegrationInputParams{"secret": jx.Raw(`42`)})},
		"a param the schema does not know": {Params: externalapi.NewOptUpdateIntegrationInputParams(externalapi.UpdateIntegrationInputParams{
			"secret": jx.Raw(strconv.Quote(newSecret("x-0123456789"))), "extra": jx.Raw(`true`),
		})},
		"a Preset that does not exist": {Preset: externalapi.NewOptNilString("nope")},
	} {
		res, err := c.IntegrationsUpdate(t.Context(), &in, externalapi.IntegrationsUpdateParams{ID: id})
		require.NoError(t, err, name)
		_, unprocessable := res.(*externalapi.IntegrationsUpdateUnprocessableEntity)
		assert.True(t, unprocessable, "%s: got %T", name, res)
	}
}

// Choosing a Preset carries nothing over from the last one: after the Preset is removed,
// choosing it again needs its params in full, secret included, and the old URL goes quiet.
func TestChoosingAPresetAgainNeedsItsParamsInFull(t *testing.T) {
	env := setupHooks(t)
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	id := int64(fixtures.IntegrationAcmeGenericAID)
	key := fixtures.IntegrationAcmeGenericAIngestKey

	removed := updateParams(t, c, id, externalapi.UpdateIntegrationInput{Preset: externalapi.OptNilString{Set: true, Null: true}})
	assert.True(t, removed.Preset.Null)
	assert.Equal(t, http.StatusNotFound, signedPost(t, env, key, secretA, "msg_1", failureBody("evt_1", "a@example.com", true)).Code,
		"an SMTP Integration with no Preset receives nothing")

	res, err := c.IntegrationsUpdate(t.Context(), &externalapi.UpdateIntegrationInput{Preset: externalapi.NewOptNilString("generic")},
		externalapi.IntegrationsUpdateParams{ID: externalapi.EntityId(id)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsUpdateUnprocessableEntity{}, res)

	updateParams(t, c, id, externalapi.UpdateIntegrationInput{Preset: externalapi.NewOptNilString("generic"), Params: secretParam(secretA)})
	assert.Equal(t, http.StatusOK, signedPost(t, env, key, secretA, "msg_2", failureBody("evt_1", "a@example.com", true)).Code)

	// Both changes of the Preset are in the Audit log, the secret in neither.
	var audit []*events.AuditEntry
	for _, ev := range env.OutboxEvents(t, events.NameAuditEntry) {
		e, ok := ev.(*events.AuditEntry)
		require.Truef(t, ok, "got %T", ev)
		audit = append(audit, e)
		raw, err := json.Marshal(e)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), secretA)
	}
	presetChanges := lo.Filter(audit, func(e *events.AuditEntry, _ int) bool { return e.Diff["preset"] != nil })
	assert.Len(t, presetChanges, 2)
}

// An agent learns what an Integration's params take from presets_list.
func TestPresetsListCarriesTheParamsSchemaOfEachPreset(t *testing.T) {
	env := setupHooks(t)
	c := env.ExternalScoped(t, "integrations:read")

	res, err := c.PresetsList(t.Context())
	require.NoError(t, err)
	list, ok := res.(*externalapi.PresetsListOK)
	require.Truef(t, ok, "got %T", res)
	generic, found := lo.Find(list.Items, func(p externalapi.PresetResource) bool { return p.Name == "generic" })
	require.True(t, found)
	assert.Contains(t, string(generic.ParamsSchema["properties"]), "writeOnly")
}

// Choosing a Preset and rotating a secret are audited, and the secret never reaches the log.
func TestChoosingAPresetAndRotatingASecretAreAuditedWithoutTheSecret(t *testing.T) {
	env := setupHooks(t)
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	id := int64(fixtures.IntegrationAcmeGenericAID)
	rotated := newSecret("audited-0123456789")

	updateParams(t, c, id, externalapi.UpdateIntegrationInput{Params: secretParam(rotated)})
	updateParams(t, c, id, externalapi.UpdateIntegrationInput{Preset: externalapi.NewOptNilString("generic"), Params: secretParam(newSecret("again-0123456789"))})

	entries := env.OutboxEvents(t, events.NameAuditEntry)
	require.NotEmpty(t, entries)
	for _, ev := range entries {
		e, ok := ev.(*events.AuditEntry)
		require.Truef(t, ok, "got %T", ev)
		assert.Equal(t, "integration.update", e.Action)
		assert.Equal(t, "changed", e.Diff["params_encrypted"], "the entry says only that the params changed")
		raw, err := json.Marshal(e)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), rotated)
		assert.NotContains(t, string(raw), secretA)
	}
}

// The hook URL rides every read of an Integration.
func TestEveryReadOfAnIntegrationCarriesItsHookURL(t *testing.T) {
	env := setupHooks(t)
	c := env.ExternalScoped(t, "integrations:read")

	res, err := c.IntegrationsGet(t.Context(), externalapi.IntegrationsGetParams{ID: externalapi.EntityId(fixtures.IntegrationAcmeGenericAID)})
	require.NoError(t, err)
	got, ok := res.(*externalapi.IntegrationResource)
	require.Truef(t, ok, "got %T", res)
	assert.True(t, strings.HasSuffix(got.HookUrl, "/hooks/"+fixtures.IntegrationAcmeGenericAIngestKey), got.HookUrl)
	preset, _ := got.Preset.Get()
	assert.Equal(t, "generic", preset)

	list, err := c.IntegrationsList(t.Context(), externalapi.IntegrationsListParams{})
	require.NoError(t, err)
	page, ok := list.(*externalapi.IntegrationsListOK)
	require.Truef(t, ok, "got %T", list)
	for _, item := range page.Items {
		assert.Contains(t, item.HookUrl, "/hooks/omik_")
	}
}
