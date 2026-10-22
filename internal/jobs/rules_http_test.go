package jobs_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/webhook"
)

const testSigner = "dGVzdC1zZWNyZXQ="

func ruleRunners(env *testhelper.TestEnv, client webhook.Doer) rules.ActionRunners {
	return jobs.RuleActionRunnersWithoutWait(newMod(env, nil), rules.New(env.Cipher, env.Bus), env.Bus, client)
}

// fireHTTPRule creates and activates a Rule that calls rcv and then applies a tag, fires it for
// Bob with the Event's properties and runs its Actions through client.
func fireHTTPRule(t *testing.T, env *testhelper.TestEnv, rcv *hookReceiver, client webhook.Doer, body string, props map[string]any) *ent.RuleFiring {
	t.Helper()
	ctx := context.Background()
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
	require.NoError(t, m.RunActions(ctx, s, fired[0].ID, ruleRunners(env, client)))
	return env.DB.RuleFiring.GetX(ctx, fired[0].ID)
}

func TestAnHTTPActionPostsTheContactAndTheEvent(t *testing.T) {
	env := testhelper.Setup(t)
	rcv := newHookReceiver(t)
	body := `{"email":"{{ email }}","event":"{{ event.action }}","plan":"{{ event.properties.plan }}","missing":"[{{ event.properties.nope }}]"}`

	f := fireHTTPRule(t, env, rcv, webhook.NewClientAllowing(2*time.Second, rcv.allow), body, map[string]any{"plan": `pro "plus"`})

	calls := rcv.received()
	require.Len(t, calls, 1)
	assert.JSONEq(t, `{"email":"bob@example.com","event":"rule_http","plan":"pro \"plus\"","missing":"[]"}`, calls[0].body)
	assert.Equal(t, "Bearer hunter2", calls[0].header.Get("Authorization"))
	assert.NotEmpty(t, calls[0].header.Get(standardwebhooks.HeaderWebhookSignature), "a secret signs the call")
	results := resultsOf(t, f)
	require.Len(t, results, 2)
	assert.Equal(t, rules.Result{Type: rules.ActionHTTP, Status: rules.Succeeded}, results[0])
	assert.Equal(t, rules.Succeeded, results[1].Status)
}

func TestAnHTTPActionRetriesAServerErrorAndSucceeds(t *testing.T) {
	env := testhelper.Setup(t)
	rcv := newHookReceiver(t)
	rcv.respond(http.StatusServiceUnavailable, "try later", "")
	rcv.onCall = func() {
		if len(rcv.received()) == 1 { // the second call finds the receiver recovered
			rcv.respond(http.StatusOK, "", "")
		}
	}

	f := fireHTTPRule(t, env, rcv, webhook.NewClientAllowing(2*time.Second, rcv.allow), `{}`, nil)

	assert.Len(t, rcv.received(), 2)
	assert.Equal(t, rules.Succeeded, resultsOf(t, f)[0].Status)
}

func TestAnHTTPActionThatKeepsFailingRecordsTheStatusAndTheAnswerAndTheNextActionStillRuns(t *testing.T) {
	env := testhelper.Setup(t)
	rcv := newHookReceiver(t)
	rcv.respond(http.StatusInternalServerError, "boom", "")

	f := fireHTTPRule(t, env, rcv, webhook.NewClientAllowing(2*time.Second, rcv.allow), `{}`, nil)

	assert.Len(t, rcv.received(), 3, "the call is tried three times")
	results := resultsOf(t, f)
	require.Len(t, results, 2)
	assert.Equal(t, rules.Failed, results[0].Status)
	assert.Equal(t, "http_status 500: boom", results[0].Detail)
	assert.Equal(t, rules.Succeeded, results[1].Status, "a failed Action does not cancel the others")
	assert.NotNil(t, f.CompletedAt)
}

func TestAnHTTPActionKeepsOnlyTheFirstKiBOfTheAnswer(t *testing.T) {
	env := testhelper.Setup(t)
	rcv := newHookReceiver(t)
	rcv.respond(http.StatusBadRequest, longAnswer(), "")

	f := fireHTTPRule(t, env, rcv, webhook.NewClientAllowing(2*time.Second, rcv.allow), `{}`, nil)

	assert.Len(t, rcv.received(), 1, "a 4xx would answer the same: no retry")
	assert.Len(t, resultsOf(t, f)[0].Detail, len("http_status 400: ")+webhook.ExcerptBytes)
}

func longAnswer() string {
	b := make([]byte, 5000)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}

func TestAnHTTPActionDoesNotCallABlockedAddress(t *testing.T) {
	env := testhelper.Setup(t)
	rcv := newHookReceiver(t)

	// The Instance allowlist does not name the receiver's loopback address.
	f := fireHTTPRule(t, env, rcv, webhook.NewClient(2*time.Second), `{}`, nil)

	assert.Empty(t, rcv.received())
	results := resultsOf(t, f)
	assert.Equal(t, rules.Failed, results[0].Status)
	assert.Equal(t, string(webhook.KindBlockedAddress), results[0].Detail)
	assert.NotContains(t, results[0].Detail, rcv.srv.URL, "the URL is a secret")
}

func TestAnHTTPActionWithABodyThatDoesNotRenderFailsWithoutCalling(t *testing.T) {
	env := testhelper.Setup(t)
	rcv := newHookReceiver(t)
	// Valid as saved (no Contact, no Event), invalid once the property is not a number.
	body := `{"n":{{ event.properties.n | default: 0 }}}`

	f := fireHTTPRule(t, env, rcv, webhook.NewClientAllowing(2*time.Second, rcv.allow), body, map[string]any{"n": "not a number"})

	assert.Empty(t, rcv.received())
	results := resultsOf(t, f)
	assert.Equal(t, rules.Failed, results[0].Status)
	assert.Contains(t, results[0].Detail, "invalid_body")
}
