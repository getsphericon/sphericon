package automations_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The kept Event (ADR 0136): a Wait for event that takes `arrived` on an Event keeps a copy of it,
// which the Steps after it bind as `awaited.<name>`, beside `event`, the triggering Event.

// awaitedTrigger and awaitedAction are what the Automations of these tests start on and await.
const (
	awaitedTrigger = "trial.started"
	awaitedAction  = "order.created"
)

// awaitedFlow is a live Automation, on an env whose clock the test moves, whose first Step waits an
// hour for an order under the name `purchase`, both its paths empty, followed by the Steps then.
type awaitedFlow struct {
	env          *testhelper.TestEnv
	clock        *testhelper.Clock
	automationID int64
	wait         automationstep.ID
}

// awaitedEnv is an env whose clock the test moves.
func awaitedEnv(t *testing.T) (*testhelper.TestEnv, *testhelper.Clock) {
	t.Helper()
	clock := testhelper.NewClock(time.Now().UTC().Truncate(time.Second))
	return testhelper.Setup(t, testhelper.WithClock(clock.Now)), clock
}

// awaitThen publishes the Automation of awaitedFlow on an env of its own.
func awaitThen(t *testing.T, then ...automationstep.Payload) awaitedFlow {
	t.Helper()
	env, clock := awaitedEnv(t)
	return awaitThenOn(t, env, clock, ending{}, then...)
}

// awaitThenOn publishes the Automation of awaitedFlow on env, whose clock is clock, ended by the
// matchers of e.
func awaitThenOn(t *testing.T, env *testhelper.TestEnv, clock *testhelper.Clock, e ending, then ...automationstep.Payload) awaitedFlow {
	t.Helper()
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	wait := automationstep.WaitForEvent{
		Name: "purchase", TimeoutSeconds: int(waitTimeout / time.Second),
		Awaits:  automationstep.AwaitedEvent{Matchers: []eventmatcher.Matcher{{Action: awaitedAction}}},
		Arrived: automationstep.Root(), TimedOut: automationstep.Root(),
	}
	a, err := m.Create(ctx, s, automations.CreateInput{
		Name: "Thanks for the purchase", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: awaitedTrigger}},
		ExitEvents: e.exits, GoalEvent: e.goal,
		Steps: steps(append([]automationstep.Payload{wait}, then...)...),
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))
	require.NoError(t, err)
	live, err := m.Get(ctx, s, a.ID)
	require.NoError(t, err)
	version, _ := automations.LiveAndDraft(live)
	return awaitedFlow{env: env, clock: clock, automationID: a.ID, wait: version.Steps.Steps[0].ID}
}

// enroll enrolls the Contact on the Trigger's Event, occurred now, and returns its Enrollment,
// standing on the Wait for event.
func (f awaitedFlow) enroll(t *testing.T, c *ent.Contact) int64 {
	t.Helper()
	handStored(t, f.env, c.ID, awaitedTrigger, collected(c.ID, awaitedTrigger, f.clock.Now(), map[string]any{"plan": "pro"}))
	runs := runsOf(t, f.env, f.automationID, c.ID)
	require.Len(t, runs, 1)
	return runs[0].ID
}

// order records the Contact's order of product, occurred now.
func (f awaitedFlow) order(t *testing.T, c *ent.Contact, product string) {
	t.Helper()
	handStored(t, f.env, c.ID, awaitedAction, collected(c.ID, awaitedAction, f.clock.Now(), map[string]any{"product": product}))
}

// run drives the queued Steps on the env's clock until none is left, unfenced: the env's one
// connection holds the test's transaction.
func (f awaitedFlow) run(t *testing.T) {
	t.Helper()
	f.env.RunQueuedSteps(t, automations.NewStepExecution(f.env.Bus, f.env.OutboundModule(), testhelper.Module[*contacts.Module](f.env), noFence, f.env.Cipher, nil, f.clock.Now))
}

// thanks is an email that names the awaited order's product and action and the triggering Event.
func thanks() automationstep.Email {
	return automationstep.Email{
		Subject: "Thanks",
		Body:    "<p>Thanks for [{{ awaited.purchase.properties.product }}] of [{{ awaited.purchase.action }}], since {{ event.action }} on {{ event.properties.plan }}</p>",
	}
}

// An email after the Wait for event prints the product of the order it arrived on. After
// `timed_out` it prints nothing for it, and `event` still prints the triggering Event.
func TestAnEmailAfterTheWaitPrintsTheAwaitedEvent(t *testing.T) {
	for name, c := range map[string]struct {
		order  func(t *testing.T, f awaitedFlow, c *ent.Contact)
		prints string
	}{
		"arrived": {func(t *testing.T, f awaitedFlow, c *ent.Contact) {
			t.Helper()
			f.clock.Advance(time.Minute)
			f.order(t, c, "Lamp")
		}, "Thanks for [Lamp] of [order.created], since trial.started on pro"},
		"timed out": {func(_ *testing.T, f awaitedFlow, _ *ent.Contact) {
			f.clock.Advance(waitTimeout)
		}, "Thanks for [] of [], since trial.started on pro"},
	} {
		t.Run(name, func(t *testing.T) {
			f := awaitThen(t, thanks())
			contact := enrollable(t, f.env.DB.Scoped(fixtures.AcmeID), 0)
			f.enroll(t, contact)

			c.order(t, f, contact)
			f.run(t)

			sent := f.env.CustomerMail.To(lo.FromPtr(contact.Email))
			require.Len(t, sent, 1)
			assert.Contains(t, sent[0].Text, c.prints)
		})
	}
}

// A text and a Telegram message after the Wait for event print the awaited order's product, as an
// email does, and `event` still prints the triggering Event.
func TestATextAndATelegramMessageAfterTheWaitPrintTheAwaitedEvent(t *testing.T) {
	const text = "Thanks for {{ awaited.purchase.properties.product }}, since {{ event.action }}"
	t.Run("sms", func(t *testing.T) {
		env, clock := awaitedEnv(t)
		account := env.AcmeTwilio(t)
		id := int64(account.Integration.ID)
		f := awaitThenOn(t, env, clock, ending{}, automationstep.SMS{IntegrationID: &id, Text: text})
		contact, err := env.DB.Contact.Get(t.Context(), fixtures.ContactTexterConfirmedID)
		require.NoError(t, err)
		f.enroll(t, contact)

		f.clock.Advance(time.Minute)
		f.order(t, contact, "Lamp")
		f.run(t)

		sent := env.Twilio.Messages()
		require.Len(t, sent, 1)
		assert.Equal(t, "Thanks for Lamp, since trial.started", sent[0].Body)
	})
	t.Run("telegram", func(t *testing.T) {
		env, clock := awaitedEnv(t)
		bot := env.AcmeTelegram(t)
		env.BindTelegram(t, bot, fixtures.ContactBobID, 555001, 555001)
		id := int64(bot.Integration.ID)
		f := awaitThenOn(t, env, clock, ending{}, automationstep.Telegram{IntegrationID: &id, Text: text})
		contact, err := env.DB.Contact.Get(t.Context(), fixtures.ContactBobID)
		require.NoError(t, err)
		f.enroll(t, contact)

		f.clock.Advance(time.Minute)
		f.order(t, contact, "Lamp")
		f.run(t)

		sends := env.Telegram.RequestsOf(telegramtest.SendMessage)
		require.Len(t, sends, 1)
		assert.Equal(t, "Thanks for Lamp, since trial.started", sends[0].String("text"))
	})
}

// capturedCall is a hook that answers 200 and keeps the body of the call it got.
type capturedCall struct{ body *string }

func (c capturedCall) Do(req *http.Request) (*http.Response, error) {
	b, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	*c.body = string(b)
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
}

// An HTTP step after the Wait for event sends the awaited order's product in its body. After
// `timed_out` the property is empty, and `event` is still the triggering Event.
func TestAnHTTPStepAfterTheWaitSendsTheAwaitedEvent(t *testing.T) {
	for name, c := range map[string]struct {
		order func(t *testing.T, f awaitedFlow, c *ent.Contact)
		sends string
	}{
		"arrived": {func(t *testing.T, f awaitedFlow, c *ent.Contact) {
			t.Helper()
			f.clock.Advance(time.Minute)
			f.order(t, c, `Lamp "XL"`)
		}, `{"product": "Lamp \"XL\"", "trigger": "trial.started"}`},
		"timed out": {func(_ *testing.T, f awaitedFlow, _ *ent.Contact) {
			f.clock.Advance(waitTimeout)
		}, `{"product": "", "trigger": "trial.started"}`},
	} {
		t.Run(name, func(t *testing.T) {
			f := awaitThen(t, automationstep.HTTP{
				URL:  "https://hooks.example.com/orders",
				Body: `{"product": "{{ awaited.purchase.properties.product }}", "trigger": "{{ event.action }}"}`,
			})
			contact := enrollable(t, f.env.DB.Scoped(fixtures.AcmeID), 0)
			runID := f.enroll(t, contact)
			c.order(t, f, contact)
			f.run(t)
			var body string
			stepper := automations.NewStepExecution(f.env.Bus, f.env.OutboundModule(), testhelper.Module[*contacts.Module](f.env), noFence, f.env.Cipher, capturedCall{body: &body}, f.clock.Now)

			_, err := stepper.ExecuteHTTP(t.Context(), f.env.DB.Scoped(fixtures.AcmeID), automations.ExecuteHTTPInput{
				RunID: runID, StepID: f.env.CurrentStepID(t, runID), Attempt: httpcall.Attempt{Number: 1, Limit: 3, CreatedAt: f.clock.Now()},
			})

			require.NoError(t, err)
			assert.JSONEq(t, c.sends, body)
		})
	}
}

// The timeout's job, finding orders that occurred within the window but were recorded late, takes
// `arrived` on the first to occur and keeps it, whichever was recorded first.
func TestTheTimeoutKeepsTheFirstEventToOccur(t *testing.T) {
	f := awaitThen(t, thanks())
	contact := enrollable(t, f.env.DB.Scoped(fixtures.AcmeID), 0)
	f.enroll(t, contact)
	deadline := f.clock.Now().Add(waitTimeout)

	f.clock.Set(deadline.Add(5 * time.Minute))
	// Recorded, their trigger jobs not yet run: the later one first.
	for _, order := range []struct {
		product string
		at      time.Time
	}{{"Second", deadline.Add(-time.Minute)}, {"First", deadline.Add(-30 * time.Minute)}} {
		require.NoError(t, f.env.Publisher().Publish(t.Context(), collected(contact.ID, awaitedAction, order.at, map[string]any{"product": order.product})))
	}
	f.run(t)

	sent := f.env.CustomerMail.To(lo.FromPtr(contact.Email))
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0].Text, "Thanks for [First] of [order.created]")
}

// keptOf is the copy the Enrollment's record of the Wait for event keeps, its action nil when it
// keeps none, and the path the record says it took.
func (f awaitedFlow) keptOf(t *testing.T, runID int64) (action *string, path automationstep.ID) {
	t.Helper()
	records, err := testhelper.Module[*automations.Module](f.env).StepRecords(t.Context(), f.env.DB.Scoped(fixtures.AcmeID), f.automationID, runID)
	require.NoError(t, err)
	waits := lo.Filter(records, func(r *ent.AutomationStepRecord, _ int) bool { return r.StepID == f.wait })
	require.Len(t, waits, 1)
	return waits[0].AwaitedAction, lo.FromPtr(waits[0].PathID)
}

// arrivedPath is the id of the Wait for event's `arrived` path.
func (f awaitedFlow) arrivedPath(t *testing.T) automationstep.ID {
	t.Helper()
	live, err := testhelper.Module[*automations.Module](f.env).Get(t.Context(), f.env.DB.Scoped(fixtures.AcmeID), f.automationID)
	require.NoError(t, err)
	version, _ := automations.LiveAndDraft(live)
	return version.Steps.Steps[0].Payload.(automationstep.WaitForEvent).Arrived.ID
}

// callHook makes the call of the HTTP step the Enrollment stands on, the hook answering status.
func (f awaitedFlow) callHook(t *testing.T, runID int64, status int) {
	t.Helper()
	stepper := automations.NewStepExecution(f.env.Bus, f.env.OutboundModule(), testhelper.Module[*contacts.Module](f.env), noFence, f.env.Cipher, httpResponse{status: status}, f.clock.Now)
	_, err := stepper.ExecuteHTTP(t.Context(), f.env.DB.Scoped(fixtures.AcmeID), automations.ExecuteHTTPInput{
		RunID: runID, StepID: f.env.CurrentStepID(t, runID), Attempt: httpcall.Attempt{Number: 1, Limit: 3, CreatedAt: f.clock.Now()},
	})
	require.NoError(t, err)
}

// The copy is kept while the Enrollment runs and cleared in the move that ends it, whatever its
// end: the path the record says it took stays.
func TestTheKeptEventIsClearedWhenTheEnrollmentEnds(t *testing.T) {
	const cancelled, upgraded = "subscription.cancelled", "plan.upgraded"
	for name, c := range map[string]struct {
		end  func(t *testing.T, f awaitedFlow, c *ent.Contact, runID int64)
		ends automationrun.Status
	}{
		"an exit event": {func(t *testing.T, f awaitedFlow, c *ent.Contact, _ int64) {
			t.Helper()
			handStored(t, f.env, c.ID, cancelled, collected(c.ID, cancelled, f.clock.Now(), nil))
		}, automationrun.StatusExited},
		"the goal event": {func(t *testing.T, f awaitedFlow, c *ent.Contact, _ int64) {
			t.Helper()
			handStored(t, f.env, c.ID, upgraded, collected(c.ID, upgraded, f.clock.Now(), nil))
		}, automationrun.StatusGoalReached},
		"a refused call": {func(t *testing.T, f awaitedFlow, _ *ent.Contact, runID int64) {
			t.Helper()
			f.clock.Advance(2 * time.Hour)
			f.run(t)
			f.callHook(t, runID, http.StatusNotFound)
		}, automationrun.StatusFailed},
		"the last step done": {func(t *testing.T, f awaitedFlow, _ *ent.Contact, runID int64) {
			t.Helper()
			f.clock.Advance(2 * time.Hour)
			f.run(t)
			f.callHook(t, runID, http.StatusOK)
		}, automationrun.StatusCompleted},
	} {
		t.Run(name, func(t *testing.T) {
			env, clock := awaitedEnv(t)
			f := awaitThenOn(t, env, clock, ending{exits: []eventmatcher.Matcher{{Action: cancelled}}, goal: &eventmatcher.Matcher{Action: upgraded}},
				automationstep.Wait{Seconds: 3600}, automationstep.HTTP{URL: "https://hooks.example.com/orders", Body: "{}"})
			contact := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
			runID := f.enroll(t, contact)
			f.clock.Advance(time.Minute)
			f.order(t, contact, "Lamp")
			whileRunning, _ := f.keptOf(t, runID)

			c.end(t, f, contact, runID)

			assert.Equal(t, c.ends, statusOf(t, env, runID))
			afterTheEnd, path := f.keptOf(t, runID)
			assert.Equal(t, awaitedAction, lo.FromPtr(whileRunning), "kept while it runs")
			assert.Nil(t, afterTheEnd, "cleared when it ends")
			assert.Equal(t, f.arrivedPath(t), path, "the path taken stays")
		})
	}
}

// A Wait for event that is the Enrollment's last Step completes it as it arrives, and keeps no
// copy: nothing comes after it to read one.
func TestAWaitForEventThatCompletesTheEnrollmentKeepsNoCopy(t *testing.T) {
	f := awaitThen(t)
	contact := enrollable(t, f.env.DB.Scoped(fixtures.AcmeID), 0)
	runID := f.enroll(t, contact)

	f.clock.Advance(time.Minute)
	f.order(t, contact, "Lamp")

	assert.Equal(t, automationrun.StatusCompleted, statusOf(t, f.env, runID))
	kept, path := f.keptOf(t, runID)
	assert.Nil(t, kept)
	assert.Equal(t, f.arrivedPath(t), path)
}
