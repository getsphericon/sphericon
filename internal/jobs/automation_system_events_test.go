package jobs_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// enrollmentEvents counts the System events of the Contact's Enrollment in the Automation, by
// action (#897).
func enrollmentEvents(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) map[string]int {
	t.Helper()
	out := map[string]int{}
	messages := env.Outbox(t,
		events.NameAutomationEnrolled, events.NameAutomationCompleted,
		events.NameAutomationGoalReached, events.NameAutomationExited)
	for i := range messages {
		m := &messages[i]
		if m.Data["automationId"] == jsonID(automationID) && m.Data["contactId"] == jsonID(contactID) {
			out[m.Envelope.Name]++
		}
	}
	return out
}

// jsonID is an id as the outbox payload decodes it.
func jsonID(id int64) json.Number { return json.Number(strconv.FormatInt(id, 10)) }

func TestAnEnrollmentRecordsItsBeginningAndItsCompletion(t *testing.T) {
	env := testhelper.Setup(t)
	enrollVia(t, env, fixtures.AutomationJobsGreetingID, fixtures.ContactAliceID)
	began := enrollmentEvents(t, env, fixtures.AutomationJobsGreetingID, fixtures.ContactAliceID)

	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(&fakeSender{})))

	assert.Equal(t, map[string]int{events.NameAutomationEnrolled: 1}, began)
	assert.Equal(t, map[string]int{events.NameAutomationEnrolled: 1, events.NameAutomationCompleted: 1},
		enrollmentEvents(t, env, fixtures.AutomationJobsGreetingID, fixtures.ContactAliceID))
}

// The Payment recovery fixture's live version: trigger payment_failed, exit event account_closed,
// goal event payment_succeeded.
func TestAnEnrollmentRecordsReachingItsGoalAndLeaving(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	on := func(contactID int64, action string) {
		t.Helper()
		require.NoError(t, stepping.OnEvent(ctx, env.Bus, acme, contactID, automations.Event{Action: action, OccurredAt: time.Now()}, 0))
	}

	on(fixtures.ContactAliceID, "payment_failed")
	on(fixtures.ContactAliceID, "payment_succeeded")
	on(fixtures.ContactBobID, "payment_failed")
	on(fixtures.ContactBobID, "account_closed")

	assert.Equal(t, map[string]int{events.NameAutomationEnrolled: 1, events.NameAutomationGoalReached: 1},
		enrollmentEvents(t, env, fixtures.AutomationPaymentRecoveryID, fixtures.ContactAliceID))
	assert.Equal(t, map[string]int{events.NameAutomationEnrolled: 1, events.NameAutomationExited: 1},
		enrollmentEvents(t, env, fixtures.AutomationPaymentRecoveryID, fixtures.ContactBobID))
}

// A failed Enrollment is an operational failure, not something the Contact did: it records no
// System event of its end.
func TestAFailedEnrollmentRecordsNoEnd(t *testing.T) {
	h := newHTTPStepHarness(t, `{"ok":true}`, true)
	runID := h.enroll(t, fixtures.ContactAliceID)
	h.rcv.respond(http.StatusNotFound, "gone", "")

	require.Error(t, h.attempt(h.worker(h.rcv.allow), runID, 1))

	assert.Equal(t, map[string]int{events.NameAutomationEnrolled: 1},
		enrollmentEvents(t, h.TestEnv, fixtures.AutomationNotifySlackID, fixtures.ContactAliceID))
}
