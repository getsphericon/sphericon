package outboundmessages_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/sending/frequencycap"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/postgres"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Automation send over the Frequency cap (ADR 0137, ADR 0178) is deferred until a place frees,
// for one period at most from when its wait at the step began; at that bound it is Skipped. Acme's
// email is capped per test (capEmail) and the counted messages are Broadcast emails sent through
// the module under the test's clock.

// automationEmailTo is the send of the Welcome series' Step stepID to the Contact, as an Enrollment
// that has waited at the Step as waited says asks for it.
func automationEmailTo(t *testing.T, env *testhelper.TestEnv, contactID int64, stepID automationstep.ID, waited outboundmessages.Waited) outboundmessages.Request {
	t.Helper()
	ctx := t.Context()
	automation := env.DB.Automation.GetX(ctx, fixtures.AutomationWelcomeSeriesID)
	run := env.DB.AutomationRun.GetX(ctx, fixtures.AutomationRunWelcomeCompletedID)
	c := env.DB.Contact.GetX(ctx, contactID)
	return outboundmessages.AutomationStep(automation, run, stepID, nil, c, outboundmessages.EmailContent{Subject: "Hi", Body: htmlBody}, waited)
}

// sendTo sends req through m and requires no error.
func sendTo(t *testing.T, m *outboundmessages.Module, env *testhelper.TestEnv, req outboundmessages.Request) outboundmessages.Result {
	t.Helper()
	res, err := m.Send(t.Context(), acme(env), req)
	require.NoError(t, err)
	return res
}

// keyHolds reports whether an Outbound message of Acme is recorded under key.
func keyHolds(t *testing.T, env *testhelper.TestEnv, key string) bool {
	t.Helper()
	return env.DB.OutboundMessage.Query().Where(outboundmessage.WorkspaceID(fixtures.AcmeID), outboundmessage.IdempotencyKey(key)).ExistX(t.Context())
}

// Alice got three Broadcast emails an hour apart under a cap of three a day: an Automation email
// three hours after the first waits until the first is a day old, and then goes.
func TestAnAutomationSendOverTheCapIsDeferredUntilThePlaceFrees(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, c.Now)
	capEmail(t, env, 3)
	start := c.Now()
	for i := range 3 {
		require.Equal(t, outboundmessages.Sent, send(t, m, env, fmt.Sprintf("defer:seed:%d", i)).Outcome)
		c.Advance(time.Hour)
	}
	req := automationEmailTo(t, env, fixtures.ContactAliceID, "1f0c3a9e-6f4b-5d2a-9c1e-7b8d0e2f4a61", outboundmessages.Waited{})

	deferred := sendTo(t, m, env, req)

	assert.Equal(t, outboundmessages.Deferral, deferred.Outcome)
	assert.Equal(t, deferralcause.FrequencyCap, deferred.Cause)
	assert.WithinDuration(t, start.Add(24*time.Hour), deferred.Until, time.Millisecond, "until the oldest of the three is a day old")
	assert.False(t, keyHolds(t, env, req.Key), "a Deferral records no message")
	assert.Equal(t, 3, counted(t, env, fixtures.AcmeID, channels.Email, fixtures.ContactAliceEmail), "and takes no place")

	c.Set(deferred.Until)
	sent := sendTo(t, m, env, req)

	assert.Equal(t, outboundmessages.Sent, sent.Outcome)
}

// Alice's wait began before three newer Broadcast emails took every place that freed: the place
// her send needs frees only past one day after the wait began, so it is deferred to that bound and,
// still over the cap there, Skipped with frequency_cap, which its message records with the wait.
func TestAnAutomationSendStillOverTheCapAtItsBoundIsSkipped(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, c.Now)
	capEmail(t, env, 3)
	since := c.Now()
	for i := range 3 {
		c.Advance(time.Hour)
		require.Equal(t, outboundmessages.Sent, send(t, m, env, fmt.Sprintf("bound:seed:%d", i)).Outcome)
	}
	c.Advance(time.Hour)
	req := automationEmailTo(t, env, fixtures.ContactAliceID, "2a7e4c1b-8d3f-5e6a-b0c2-9f1d3e5a7b82", outboundmessages.Waited{Since: since, Cause: deferralcause.FrequencyCap})

	deferred := sendTo(t, m, env, req)
	c.Set(deferred.Until)
	skipped := sendTo(t, m, env, req)

	assert.Equal(t, outboundmessages.Deferral, deferred.Outcome)
	assert.WithinDuration(t, since.Add(24*time.Hour), deferred.Until, time.Millisecond, "the bound comes before the first place frees")
	assert.Equal(t, outboundmessages.Skipped, skipped.Outcome)
	assert.Equal(t, skipreason.FrequencyCap, skipped.SkippedReason)
	msg := byKey(t, env, req.Key)
	assert.Equal(t, skipreason.FrequencyCap, lo.FromPtr(msg.SkippedReason))
	require.NotNil(t, msg.DeferredSince)
	assert.WithinDuration(t, since, *msg.DeferredSince, time.Microsecond, "the message keeps since when it waited")
	assert.Equal(t, deferralcause.FrequencyCap, lo.FromPtr(msg.DeferralCause))
}

// A final Outcome outranks a temporary one (ADR 0178): a Contact who unsubscribed from the
// Automation is Skipped for it, never deferred, though his address is at its cap.
func TestAnUnsubscribedContactsSendOverTheCapIsSkippedNotDeferred(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, c.Now)
	capEmail(t, env, 1)
	leo := env.DB.Contact.GetX(t.Context(), fixtures.ContactOptedOutOfWelcomeID)
	require.Equal(t, outboundmessages.Sent, sendTo(t, m, env, marketing(t, env, "unsub:seed", leo.ID)).Outcome)
	req := automationEmailTo(t, env, leo.ID, "3b8f5d2c-9e4a-5f7b-a1d3-0e2f4a6b8c93", outboundmessages.Waited{})

	res := sendTo(t, m, env, req)

	assert.Equal(t, outboundmessages.Skipped, res.Outcome)
	assert.Equal(t, skipreason.UnsubscribedSource, res.SkippedReason)
	assert.Equal(t, 1, counted(t, env, fixtures.AcmeID, channels.Email, *leo.Email), "the address is at its cap")
}

// A Broadcast email and an Automation email to Alice at her last place, queued behind its lock: the
// Broadcast's takes the place, and the Automation's reads it taken and is deferred, not Skipped.
func TestAnAutomationSendThatLosesTheLastPlaceIsDeferred(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	capEmail(t, env, 1)
	key := postgres.DestinationKey(frequencycap.LockKind, fixtures.AcmeID, string(channels.Email), fixtures.ContactAliceEmail)
	automation := automationEmailTo(t, env, fixtures.ContactAliceID, "4c9a6e3d-0f5b-5a8c-b2e4-1f3a5b7c9d04", outboundmessages.Waited{})
	var broadcast, automated outboundmessages.Result
	writer := func(name string, req outboundmessages.Request, res *outboundmessages.Result) testhelper.Writer {
		return testhelper.Writer{Name: name, Run: func() error {
			var err error
			*res, err = env.OutboundModule().Send(t.Context(), acme(env), req)
			return err
		}}
	}

	errs := env.QueueBehindKey(t, key,
		writer("broadcast", marketing(t, env, "race:broadcast", fixtures.ContactAliceID), &broadcast),
		writer("automation", automation, &automated))

	require.NoError(t, errs["broadcast"])
	require.NoError(t, errs["automation"])
	assert.Equal(t, outboundmessages.Sent, broadcast.Outcome)
	assert.Equal(t, outboundmessages.Deferral, automated.Outcome)
	assert.Equal(t, deferralcause.FrequencyCap, automated.Cause)
	assert.True(t, automated.Until.After(time.Now()), "until the Broadcast's place leaves the period")
	assert.False(t, keyHolds(t, env, automation.Key), "the deferred send records no message")
	assert.Equal(t, 1, counted(t, env, fixtures.AcmeID, channels.Email, fixtures.ContactAliceEmail))
}

// Two attempts of one Automation step's send to Alice at her cap, queued behind her lock: the first
// has waited past its bound and is Skipped with frequency_cap; the second, which found no message
// under the key before it waited for the lock, reads the key again under it and takes the first's
// decision instead of a Deferral of its own.
func TestAnAttemptBehindTheLockTakesTheDecisionAnotherMadeUnderItsKey(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	capEmail(t, env, 1)
	seed := sendTo(t, env.OutboundModule(), env, marketing(t, env, "reread:seed", fixtures.ContactAliceID))
	require.Equal(t, outboundmessages.Sent, seed.Outcome)
	key := postgres.DestinationKey(frequencycap.LockKind, fixtures.AcmeID, string(channels.Email), fixtures.ContactAliceEmail)
	stepID := automationstep.ID("6e2c8a5f-3b7d-5cae-d4a6-3b5c7d9e1f26")
	pastItsBound := outboundmessages.Waited{Since: time.Now().Add(-48 * time.Hour), Cause: deferralcause.FrequencyCap}
	bounded := automationEmailTo(t, env, fixtures.ContactAliceID, stepID, pastItsBound)
	fresh := automationEmailTo(t, env, fixtures.ContactAliceID, stepID, outboundmessages.Waited{})
	var first, second outboundmessages.Result
	writer := func(name string, req outboundmessages.Request, res *outboundmessages.Result) testhelper.Writer {
		return testhelper.Writer{Name: name, Run: func() error {
			var err error
			*res, err = env.OutboundModule().Send(t.Context(), acme(env), req)
			return err
		}}
	}

	errs := env.QueueBehindKey(t, key, writer("first", bounded, &first), writer("second", fresh, &second))

	require.NoError(t, errs["first"])
	require.NoError(t, errs["second"])
	assert.Equal(t, outboundmessages.Skipped, first.Outcome)
	assert.Equal(t, outboundmessages.Skipped, second.Outcome, "the second takes the decision under its key, not a Deferral")
	assert.Equal(t, skipreason.FrequencyCap, second.SkippedReason)
	assert.Equal(t, first.MessageID, second.MessageID)
}
