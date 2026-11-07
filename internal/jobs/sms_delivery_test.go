package jobs_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging/twilio/twiliotest"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
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

// Lacuna's four confirmed phones are texted: Ada's text is delivered, Bea's undelivered, and Cyd's
// and Dot's get no final report.
func TestAnSMSBroadcastsDeliveredAndUndeliveredFiguresMatchTheReports(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.LacunaTwilio(t)
	id := int64(fixtures.BroadcastLacunaSmsHelloID)
	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))

	adaText := textOf(t, env, fixtures.ContactLacunaAdaPhone)
	beaText := textOf(t, env, fixtures.ContactLacunaBeaPhone)

	reportStatus(t, env, account, adaText, "sent", "")
	reportStatus(t, env, account, adaText, "delivered", "")
	reportStatus(t, env, account, beaText, "undelivered", "30003")
	reportStatus(t, env, account, beaText, "delivered", "")

	b := env.DB.Broadcast.GetX(ctx, id)
	st, err := broadcasts.StatsFor(ctx, env.DB.Scoped(fixtures.LacunaID), b)
	require.NoError(t, err)
	assert.Equal(t, broadcasts.Stats{Channel: channels.SMS, Recipients: 4, Sent: 4, Delivered: 1, Undelivered: 1}, st[id],
		"Cyd's and Dot's texts have no final report: they are neither")
	figures := st[id].Figures()
	assert.Equal(t, 1, *figures.Delivered)
	assert.Equal(t, 1, *figures.Undelivered)
	assert.Nil(t, figures.Bounced)
	assert.Nil(t, figures.Complained)
	assert.Nil(t, figures.Opened)
	assert.Nil(t, figures.DeliveryRate)
}

// An Automation whose Trigger is sms.undelivered enrolls the Contact whose text Twilio could not
// deliver, and its steps see the error code. Fixture: Lacuna's Automation on sms.undelivered.
func TestAnAutomationTriggeredByAnUndeliveredTextEnrollsItsContact(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.LacunaTwilio(t)
	id := int64(fixtures.BroadcastLacunaSmsHelloID)
	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))

	text := textOf(t, env, fixtures.ContactLacunaAdaPhone)
	reportStatus(t, env, account, text, "undelivered", "30007")
	onlyEnvelope(t, env, events.NameSmsUndelivered)
	triggers := triggersOn(t, env, events.NameSmsUndelivered)
	require.Len(t, triggers, 1)
	require.NoError(t, jobs.NewEvaluateTriggerWorker(env.DB, env.StepExecution).Work(ctx, job(triggers[0])))

	run, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationLacunaOnSmsUndeliveredID), automationrun.ContactID(fixtures.ContactLacunaAdaID)).
		Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, events.NameSmsUndelivered, *run.EventAction)
	assert.Equal(t, "30007", run.EventProperties["errorCode"])
}
