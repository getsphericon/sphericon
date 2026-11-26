package jobs_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationskippedentry"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Entry policy `again` through the steps an Enrollment runs (ADR 0127, ADR 0081): an opt-out
// from the Automation exits the Enrollment active when it arrives, and a later Enrollment of the
// same Contact runs, its sends to the Destination that refused the Automation skipped.

// repeatOrder is the customer action the Automation of these tests starts on.
const repeatOrder = "repeat_order"

// byOrdinalOf is the Contact's Enrollments in the Automation, the first first.
func byOrdinalOf(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) []*ent.AutomationRun {
	t.Helper()
	return env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(automationID), automationrun.ContactID(contactID)).
		Order(ent.Asc(automationrun.FieldOrdinal)).
		AllX(t.Context())
}

// orderAt hands the Contact's repeatOrder, occurred at, to the Automations, as its trigger job does.
func orderAt(t *testing.T, env *testhelper.TestEnv, contactID int64, at time.Time) {
	t.Helper()
	in := automations.OnEventInput{ContactID: contactID, Event: automations.Event{Action: repeatOrder, OccurredAt: at}}
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(t.Context(), env.DB.Scoped(fixtures.AcmeID), in))
}

// Tess unsubscribed from Automation A by email while its Enrollment ran: that Enrollment exits. An
// order two hours later enrolls her again under `again`, her second Enrollment: A's email to her is
// Skipped with unsubscribed_source and the Enrollment moves on, its Tag step runs, its text to her
// phone, which did not refuse A, goes, and it completes. An order a minute later is within the
// cooldown of that Enrollment's beginning.
func TestAContactUnsubscribedFromAnAutomationReentersItUnderAgain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.AcmeTwilio(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	created, err := m.Create(ctx, s, automations.CreateInput{
		Name: "Repeat order", Trigger: automations.Trigger{Event: eventmatcher.Matcher{Action: repeatOrder}},
		Entry: automations.Entry{Policy: automationversion.EntryPolicyAgain},
		Steps: new(automationstep.Root(
			automationstep.New(automationstep.Email{Subject: "Thanks", Body: "<p>Thanks for your order.</p>"}),
			automationstep.New(automationstep.ApplyTag{Tag: "repeat-buyer"}),
			smsStep(account, "Your order is on its way"),
		)),
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, created.ID, testhelper.AutomationRevisionIn(ctx, t, s, created.ID))
	require.NoError(t, err)
	confirmSMS(t, env, fixtures.ContactTexterPhone)
	orderAt(t, env, fixtures.ContactTexterID, time.Now().Add(-2*time.Hour))
	_, err = env.ExternalScoped(t, apitokens.ScopeContactsWrite).UnsubscribesCreate(ctx, &externalapi.CreateUnsubscribeInput{
		Destination: fixtures.ContactTexterEmail, SendingSource: externalapi.NewOptString(eligibility.AutomationSource(created.ID)),
	})
	require.NoError(t, err)

	orderAt(t, env, fixtures.ContactTexterID, time.Now())
	// Its steps hold no wait: they run on the wall clock, so the Enrollment ends now.
	env.RunQueuedSteps(t, stepperAt(env, env.OutboundModule(), time.Now))
	orderAt(t, env, fixtures.ContactTexterID, time.Now().Add(time.Minute))

	runs := byOrdinalOf(t, env, created.ID, fixtures.ContactTexterID)
	require.Len(t, runs, 2)
	assert.Equal(t, automationrun.StatusExited, runs[0].Status, "the unsubscribe exited the Enrollment active when it arrived")
	second := runs[1]
	assert.Equal(t, 2, second.Ordinal)
	assert.Equal(t, automationrun.StatusCompleted, second.Status, "the opt-out does not exit an Enrollment that began after it")
	sends := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(second.ID)).AllX(ctx)
	byChannel := lo.SliceToMap(sends, func(msg *ent.OutboundMessage) (channels.Channel, *ent.OutboundMessage) { return msg.Channel, msg })
	require.Len(t, byChannel, 2)
	assert.Equal(t, outboundmessage.StatusSkipped, byChannel[channels.Email].Status)
	assert.Equal(t, skipreason.UnsubscribedSource, lo.FromPtr(byChannel[channels.Email].SkippedReason))
	assert.Equal(t, outboundmessage.StatusSent, byChannel[channels.SMS].Status)
	texts := env.Twilio.Messages()
	require.Len(t, texts, 1)
	assert.Equal(t, fixtures.ContactTexterPhone, texts[0].To)
	assert.Contains(t, tagsOf(t, env, fixtures.ContactTexterID), "repeat-buyer")
	page, err := m.SkippedEntries(ctx, s, pagination.Params{Page: 1, PageSize: 100}, automations.SkippedEntriesFilter{AutomationID: created.ID, ContactID: fixtures.ContactTexterID})
	require.NoError(t, err)
	reasons := lo.Map(page.Items, func(e *ent.AutomationSkippedEntry, _ int) automationskippedentry.Reason { return e.Reason })
	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonCooldown}, reasons)
}

// An opt-out from the Automation's own sending source skips its sends instead of sending, and the
// Enrollment moves on through its steps to the end: the opt-out is about messages (ADR 0127). Leo
// opted out of the Welcome series' own sending source.
func TestAnOptOutFromTheAutomationSkipsItsSendsAndTheEnrollmentMovesOn(t *testing.T) {
	env := testhelper.Setup(t)
	runID := enrollVia(t, env, fixtures.AutomationWelcomeSeriesID, fixtures.ContactOptedOutOfWelcomeID)

	fs := &fakeSender{}
	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(fs)))

	assert.Empty(t, fs.sent, "ineligible destination receives no email")
	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(t.Context(), runID).Status)
	records := stepRecordsOf(t, env, runID)
	require.NotEmpty(t, records)
	assert.Equal(t, skipreason.UnsubscribedSource, lo.FromPtr(records[0].SkipReason), "the send it skipped says why")
}

// Under `again` with a cooldown, a click of the Automation's own email after its Enrollment ended
// enrolls Uma once more; a click within the cooldown of that Enrollment's beginning, though it
// ended too, records `cooldown` (ADR 0053, ADR 0127).
func TestAClickOnTheAutomationsOwnEmailReentersItOncePerCooldownUnderAgain(t *testing.T) {
	env := testhelper.Setup(t)
	onClick := int64(fixtures.AutomationOuroborosOnClickID)
	onlyActive(t, env, onClick)
	env.DB.AutomationVersion.UpdateOneID(fixtures.AutomationOuroborosOnClickVersionID).SetEntryPolicy(automationversion.EntryPolicyAgain).ExecX(t.Context())
	click := func(at time.Time) *events.EmailEngagement {
		return &events.EmailEngagement{
			Action: systemevent.EmailClicked, WorkspaceID: fixtures.OuroborosID, ContactID: fixtures.ContactOuroborosUmaID,
			Email: fixtures.ContactOuroborosUmaEmail, AutomationID: onClick, ObservedAt: at,
		}
	}
	publishIn(t.Context(), t, env, click(time.Now().Add(-3*time.Hour)))
	env.PerformEnqueuedJobs(t)
	exited, err := env.ExitEnrollment(t.Context(), ouroboros(env), onClick, fixtures.ContactOuroborosUmaID)
	require.NoError(t, err)

	publishIn(t.Context(), t, env, click(time.Now()))
	env.PerformEnqueuedJobs(t)
	exitedAgain, err := env.ExitEnrollment(t.Context(), ouroboros(env), onClick, fixtures.ContactOuroborosUmaID)
	require.NoError(t, err)
	publishIn(t.Context(), t, env, click(time.Now().Add(time.Minute)))
	env.PerformEnqueuedJobs(t)

	runs := byOrdinalOf(t, env, onClick, fixtures.ContactOuroborosUmaID)
	ordinals := lo.Map(runs, func(r *ent.AutomationRun, _ int) int { return r.Ordinal })
	skips := skipsOf(t, env, onClick, fixtures.ContactOuroborosUmaID)
	assert.True(t, exited)
	assert.True(t, exitedAgain)
	assert.Equal(t, []int{1, 2}, ordinals, "the click after the end enrolled her once more")
	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonCooldown}, skips)
}
