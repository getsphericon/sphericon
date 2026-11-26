package jobs_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging/twilio/twiliotest"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Sending window (ADR 0134) on the surfaces that send: a Broadcast Recipient outside its window
// stays pending until it opens while the others go on schedule, and an Automation's text waits at
// its step. The fixture windows are open all day: each test narrows the one it reads. One clock runs the Instance, set after the real one so the fixtures' times keep their
// meaning.

// tomorrowAt is hour o'clock in loc on the day after today there.
func tomorrowAt(loc *time.Location, hour int) time.Time {
	day := time.Now().In(loc).AddDate(0, 0, 1)
	return time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, loc)
}

// windowSMS gives the scoped Workspace an SMS window from the hour opens to the hour closes every
// day in place of its windows, as its settings would.
func windowSMS(t *testing.T, env *testhelper.TestEnv, s *ent.Scoped, opens, closes int) {
	t.Helper()
	m := testhelper.Module[*workspaces.Module](env)
	rules, err := m.SendingRules(t.Context(), s)
	require.NoError(t, err)
	hours := lo.Map(localcalendar.Weekdays, func(day localcalendar.Weekday, _ int) workspaces.OpenHours {
		return workspaces.OpenHours{Day: day, OpensAt: opens * 60, ClosesAt: closes * 60}
	})
	_, err = m.ReplaceSendingRules(t.Context(), s, workspaces.ReplaceSendingRulesInput{
		Revision: rules.Revision, FrequencyCaps: rules.FrequencyCaps,
		SendingWindows: []workspaces.SendingWindow{{Channel: channels.SMS, Hours: hours}},
	})
	require.NoError(t, err)
}

// readsAt reports whether t reads as hour:00 in loc, give or take the minute of jitter a wake-up
// adds to its release moment.
func readsAt(t time.Time, loc *time.Location, hour int) bool {
	local := t.In(loc)
	return local.Hour() == hour && local.Minute() == 0
}

var (
	newYork  = lo.Must(time.LoadLocation("America/New_York"))
	honolulu = lo.Must(time.LoadLocation("Pacific/Honolulu"))
)

const dotInHonolulu = "+18085557743"

// Lacuna texts from 9:00 to 17:00, and its news Broadcast is sending to its four texters at 10:00
// in New York. Ada's Boston number is texted on schedule; Bea's had three texts today and is
// Skipped for the cap; Cyd's time zone says Tokyo, whose 9:00 to 17:00 never meets New York's, so
// she has no local time; Dot moved to a Honolulu number, where it is 5:00, so she waits. The report
// counts each, and the one send judged by a single zone. At 9:00 in Honolulu Dot's text goes too.
func TestABroadcastTextsTheRecipientsInsideTheirWindowAndTheOthersWhenItOpens(t *testing.T) {
	c := testhelper.NewClock(tomorrowAt(newYork, 10))
	env := testhelper.Setup(t, testhelper.WithClock(c.Now))
	ctx := t.Context()
	account := env.LacunaTwilio(t)
	mod := env.OutboundModule()
	lacuna := env.DB.Scoped(fixtures.LacunaID)
	id := int64(fixtures.BroadcastLacunaSmsHelloID)
	bea := env.DB.Contact.GetX(ctx, fixtures.ContactLacunaBeaID)
	integrationID := int64(account.Integration.ID)
	for i := range 3 {
		res, err := mod.Send(ctx, lacuna, outboundmessages.Request{
			Ref: outboundmessages.BroadcastRef{BroadcastID: fixtures.BroadcastLacunaSmsGreetingID}, Key: fmt.Sprintf("earlier:%d", i),
			Contact: bea, Destination: *bea.Phone, IntegrationID: &integrationID, Content: outboundmessages.SMSContent{Text: "Sale"},
		})
		require.NoError(t, err)
		require.Equal(t, outboundmessages.Sent, res.Outcome)
	}
	windowSMS(t, env, lacuna, 9, 17)
	env.DB.Contact.UpdateOneID(fixtures.ContactLacunaCydID).SetTimeZone("Asia/Tokyo").ExecX(ctx)
	env.DB.Contact.UpdateOneID(fixtures.ContactLacunaDotID).SetPhone(dotInHonolulu).ExecX(ctx)
	env.DB.Confirmation.UpdateOneID(fixtures.ConfirmationLacunaDotID).SetDestination(dotInHonolulu).ExecX(ctx)
	ids, err := planAudience(ctx, env.DB, mod, id)
	require.NoError(t, err)
	env.DB.Broadcast.UpdateOneID(id).SetStatus(broadcast.StatusSending).ExecX(ctx)
	dot := env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.BroadcastID(id), broadcastrecipient.ContactID(fixtures.ContactLacunaDotID)).OnlyX(ctx)
	errs := lo.Map(ids, func(rid int64, _ int) error { return deliverRecipient(ctx, env.DB, mod, rid) })

	deferred := lo.Filter(errs, func(err error, _ int) bool { return err != nil })
	require.Len(t, deferred, 1, "only Dot's send waits")
	waits, ok := errors.AsType[*broadcasts.DeferredError](deferred[0])
	require.True(t, ok)
	assert.Equal(t, deferralcause.SendingWindow, waits.Cause)
	texted := lo.Map(env.Twilio.Messages(), func(m twiliotest.Message, _ int) string { return m.To })
	bea3 := []string{fixtures.ContactLacunaBeaPhone, fixtures.ContactLacunaBeaPhone, fixtures.ContactLacunaBeaPhone}
	assert.Equal(t, append(bea3, fixtures.ContactLacunaAdaPhone), texted, "Bea's three earlier texts, then Ada's on schedule")
	waiting := env.DB.BroadcastRecipient.GetX(ctx, dot.ID)
	assert.Equal(t, deferralcause.SendingWindow, lo.FromPtr(waiting.DeferralCause))
	require.NotNil(t, waiting.DeferredUntil)
	assert.True(t, readsAt(*waiting.DeferredUntil, honolulu, 9), "Dot waits until 9:00 in Honolulu: %s", waiting.DeferredUntil.In(honolulu))
	figures := figuresOf(t, env, id)
	assert.Equal(t, 1, figures.Sent)
	assert.Equal(t, map[skipreason.Reason]int{skipreason.FrequencyCap: 1, skipreason.NoLocalTime: 1}, figures.SkippedBy)
	assert.Equal(t, map[deferralcause.Cause]int{deferralcause.SendingWindow: 1}, figures.Waiting)
	assert.Equal(t, 1, figures.SingleZone, "Ada's Boston number is in New York alone")

	c.Set(*waiting.DeferredUntil)
	require.NoError(t, deliverRecipient(ctx, env.DB, mod, dot.ID))

	texted = lo.Map(env.Twilio.Messages(), func(m twiliotest.Message, _ int) string { return m.To })
	assert.Equal(t, append(bea3, fixtures.ContactLacunaAdaPhone, dotInHonolulu), texted, "Dot is texted when her window opens")
	assert.Equal(t, broadcast.StatusSent, env.DB.Broadcast.GetX(ctx, id).Status)
	after := figuresOf(t, env, id)
	assert.Equal(t, 2, after.Sent)
	assert.Equal(t, 2, after.SingleZone)
	assert.Empty(t, after.Waiting)
}

// ContactTexter's Enrollment stands on a text at 22:00 in Los Angeles, his number's zone: the step
// waits, queued for 8:00 there with no more than a minute of jitter and the wait shown on the
// Enrollment; at 8:00 the text is sent and the Enrollment goes on.
func TestAnAutomationTextOutsideTheWindowWaitsAtItsStepUntilItOpens(t *testing.T) {
	losAngeles := lo.Must(time.LoadLocation("America/Los_Angeles"))
	c := testhelper.NewClock(tomorrowAt(losAngeles, 22))
	env := testhelper.Setup(t, testhelper.WithClock(c.Now))
	ctx := t.Context()
	env.AcmeTwilio(t)
	confirmSMS(t, env, fixtures.ContactTexterPhone)
	windowSMS(t, env, env.DB.Scoped(fixtures.AcmeID), 8, 20)
	stepper := stepperAt(env, env.OutboundModule(), c.Now)
	runID := int64(fixtures.AutomationRunJobsSMSHiThenTagTexterID)
	queueStep(t, env, runID)

	require.True(t, env.RunNextStep(t, stepper))

	at := env.JobSchedules(t, automations.RunStepArgs{}.Kind())
	require.Len(t, at, 1)
	due := at[0]
	assert.True(t, readsAt(due, losAngeles, 8), "queued for 8:00 in Los Angeles: %s", due.In(losAngeles))
	assert.Less(t, due.Sub(c.Now()), 12*time.Hour, "the next morning")
	waiting := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, automationrun.StatusActive, waiting.Status)
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(waiting.CurrentStepID), "the Enrollment waits at its step")
	assert.Equal(t, deferralcause.SendingWindow, lo.FromPtr(waiting.DeferralCause))
	assert.Empty(t, env.Twilio.Messages(), "nothing is texted at night")

	c.Set(due)
	env.RunQueuedSteps(t, stepper)

	texts := env.Twilio.Messages()
	require.Len(t, texts, 1)
	assert.Contains(t, texts[0].Body, "Hi")
	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status)
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(ctx)
	assert.Equal(t, deferralcause.SendingWindow, lo.FromPtr(msg.DeferralCause), "the message keeps why it waited")
	assert.True(t, msg.WindowSingleZone)
}
