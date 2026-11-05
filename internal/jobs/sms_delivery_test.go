package jobs_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/messaging/twilio/twiliotest"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Twilio's delivery reports of an SMS Broadcast's texts (ADR 0079): its figures count the texts
// reported delivered and undelivered, and an undelivered text can start an Automation.

// textOf is the text Twilio accepted for the number.
func textOf(t *testing.T, env *testhelper.TestEnv, number string) twiliotest.Message {
	t.Helper()
	for _, m := range env.Twilio.Messages() {
		if m.To == number {
			return m
		}
	}
	require.FailNowf(t, "no text", "Twilio received no text for %s", number)
	return twiliotest.Message{}
}

func reportStatus(t *testing.T, env *testhelper.TestEnv, account testhelper.TwilioAccount, msg twiliotest.Message, status, code string) {
	t.Helper()
	res := env.ReportTwilioStatus(t, account, msg, status, code)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
}

func TestAnSMSBroadcastsDeliveredAndUndeliveredFiguresMatchTheReports(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.AcmeTwilio(t)
	confirmSMS(t, env, fixtures.ContactTexterPhone)
	env.DB.Contact.UpdateOneID(fixtures.ContactAliceID).SetPhone(alicePhone).ExecX(ctx)
	confirmSMS(t, env, alicePhone)
	env.DB.Contact.UpdateOneID(fixtures.ContactBobID).SetPhone(bobPhone).ExecX(ctx)
	confirmSMS(t, env, bobPhone)
	id := smsBroadcast(t, env, account, "Sale today")
	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))

	tessText := textOf(t, env, fixtures.ContactTexterPhone)
	aliceText := textOf(t, env, alicePhone)

	reportStatus(t, env, account, tessText, "sent", "")
	reportStatus(t, env, account, tessText, "delivered", "")
	reportStatus(t, env, account, aliceText, "undelivered", "30003")
	reportStatus(t, env, account, aliceText, "delivered", "")

	b := env.DB.Broadcast.GetX(ctx, id)
	st, err := broadcasts.StatsFor(ctx, env.DB.Scoped(fixtures.AcmeID), b)
	require.NoError(t, err)
	assert.Equal(t, broadcasts.Stats{Channel: broadcast.ChannelSms, Recipients: 3, Sent: 3, Delivered: 1, Undelivered: 1}, st[id],
		"Bob's text has no final report: it is neither")
	figures := st[id].Figures()
	assert.Equal(t, 1, *figures.Delivered)
	assert.Equal(t, 1, *figures.Undelivered)
	assert.Nil(t, figures.Bounced)
	assert.Nil(t, figures.Complained)
	assert.Nil(t, figures.Opened)
	assert.Nil(t, figures.DeliveryRate)
}

// An Automation whose Trigger is sms.undelivered enrolls the Contact whose text Twilio could not
// deliver, and its steps see the error code. Fixture: the Jobs greeting Automation, its live
// version's Trigger turned to sms.undelivered.
func TestAnAutomationTriggeredByAnUndeliveredTextEnrollsItsContact(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	greeting := env.DB.Automation.Query().Where(automation.ID(fixtures.AutomationJobsGreetingID)).OnlyX(ctx)
	env.DB.AutomationVersion.UpdateOneID(*greeting.LiveVersionID).SetTriggerEvent(events.NameSmsUndelivered).ExecX(ctx)
	account := env.AcmeTwilio(t)
	confirmSMS(t, env, fixtures.ContactTexterPhone)
	id := smsBroadcast(t, env, account, "Sale today")
	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))

	text := textOf(t, env, fixtures.ContactTexterPhone)
	reportStatus(t, env, account, text, "undelivered", "30007")
	undelivered := onlyEnvelope(t, env, events.NameSmsUndelivered)
	env.WorkAutomationJobOf(t, undelivered)
	triggers := queuedArgs[jobs.EvaluateTriggerArgs](t, env, jobs.EvaluateTriggerArgs{}.Kind())
	require.Len(t, triggers, 1)
	require.NoError(t, jobs.NewEvaluateTriggerWorker(env.DB, env.Bus).Work(ctx, job(triggers[0])))

	run, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationJobsGreetingID), automationrun.ContactID(fixtures.ContactTexterID)).
		Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, events.NameSmsUndelivered, *run.EventAction)
	assert.Equal(t, "30007", run.EventProperties["errorCode"])
}
