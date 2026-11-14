package jobs_test

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A new SMS opt-out of a Contact's number is the customer-facing sms.unsubscribed, as a Telegram
// stop is telegram.unsubscribed: Webhook endpoints, Rules and Automations learn of it.

// The opt-out reaches an endpoint that selects sms.unsubscribed with the public body: the Contact,
// and in its data how the person stopped and the number.
func TestAnSMSOptOutIsDeliveredToTheEndpointsThatSelectIt(t *testing.T) {
	env := testhelper.Setup(t)
	account := env.AcmeTwilio(t)
	env.DB.WebhookEndpoint.UpdateOneID(fixtures.WebhookCodebasicsID).SetEventTypes([]string{systemevent.SmsUnsubscribed}).ExecX(t.Context())
	textIn(t, env, account, "SM0401", fixtures.ContactTexterPhone, "STOP")

	env.PerformEnqueuedJobs(t, jobs.WebhookEventArgs{})

	d := deliveryTo(t, env, fixtures.WebhookCodebasicsID)
	assert.Equal(t, systemevent.SmsUnsubscribed, d.EventName)
	body := bodyOf(t, d)
	assert.Equal(t, systemevent.SmsUnsubscribed, body["type"])
	assert.Equal(t, testhelper.ID(fixtures.ContactTexterID), body["contactId"])
	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "the body carries the Event's data")
	assert.Equal(t, events.UnsubscribedByKeyword, data["via"])
	assert.Equal(t, fixtures.ContactTexterPhone, data["phone"])
}

// An Automation may start on sms.unsubscribed: a STOP from a Contact's number enrolls the Contact,
// and its steps see how it stopped. Fixture: the Automation on sms.undelivered, whose live version
// the test turns to start on sms.unsubscribed.
func TestAnAutomationTriggeredByAnSMSOptOutEnrollsItsContact(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.AcmeTwilio(t)
	env.DB.AutomationVersion.UpdateOneID(fixtures.AutomationOnSmsUndeliveredVersionID).SetTriggerEvent(systemevent.SmsUnsubscribed).ExecX(ctx)
	textIn(t, env, account, "SM0402", fixtures.ContactTexterPhone, "STOP")
	onlyEnvelope(t, env, systemevent.SmsUnsubscribed)
	triggers := triggersOn(t, env, systemevent.SmsUnsubscribed)
	require.Len(t, triggers, 1)

	require.NoError(t, jobs.NewEvaluateTriggerWorker(env.DB, env.StepExecution).Work(ctx, job(triggers[0])))

	run, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationOnSmsUndeliveredID), automationrun.ContactID(fixtures.ContactTexterID)).
		Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, systemevent.SmsUnsubscribed, *run.EventAction)
	assert.Equal(t, events.UnsubscribedByKeyword, run.EventProperties["via"])
}

// unsubscribeLink is the token of the unsubscribe link the only text Twilio got ends with.
func unsubscribeLink(t *testing.T, env *testhelper.TestEnv) string {
	t.Helper()
	sent := env.Twilio.Messages()
	require.Len(t, sent, 1)
	link := regexp.MustCompile(`/e/u/(\S+)$`).FindStringSubmatch(sent[0].Body)
	require.Len(t, link, 2, "the text ends with the unsubscribe link")
	return link[1]
}

// The unsubscribe link of a Broadcast's text names the Broadcast on the opt-out of the Contact it
// was sent to (ADR 0115), and the Broadcast counts the opt-out once. Lacuna's draft "Sale: ... Stop:
// <unsubscribe link>" goes to Bea, Textham's one number that can be texted.
func TestAnSMSBroadcastsLinkNamesTheBroadcastAndCountsOnce(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.LacunaTwilio(t)
	id := int64(fixtures.BroadcastLacunaSmsLinksID)
	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), id))
	token := unsubscribeLink(t, env)

	err := env.SiteAnonymous(t).SitePublicUnsubscribesPerform(ctx, siteapi.SitePublicUnsubscribesPerformParams{Token: token})

	require.NoError(t, err)
	unsubscribed := env.QueuedMessages(t, systemevent.SmsUnsubscribed)
	require.Len(t, unsubscribed, 1)
	data := unsubscribed[0].Data
	assert.Equal(t, strconv.Itoa(fixtures.ContactLacunaBeaID), fmt.Sprint(data["contactId"]))
	assert.Equal(t, strconv.FormatInt(id, 10), fmt.Sprint(data["broadcastId"]))
	assert.Equal(t, events.UnsubscribedByLink, data["via"])
	assert.NotContains(t, data, "automationId")
	attributions := env.JobsOf(t, jobs.BroadcastUnsubscribeEventArgs{}.Kind())
	require.Len(t, attributions, 1, "the public Event is no second attribution fact")
	env.PerformEnqueuedJobs(t, jobs.BroadcastUnsubscribeEventArgs{})
	counted := env.DB.BroadcastRecipient.Query().Where(
		broadcastrecipient.BroadcastID(id), broadcastrecipient.ContactID(fixtures.ContactLacunaBeaID),
	).OnlyX(ctx)
	assert.NotNil(t, counted.UnsubscribedAt, "the Broadcast counts the opt-out")
}

// A number that moved to another Contact after its text went out is that Contact's opt-out when the
// link is followed, but names no Automation on it: the person who stopped received no text of it.
// The text's own attribution fact still counts the opt-out on the message (ADR 0115).
func TestAnSMSLinkOfANumberNowAnotherContactsNamesNoAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.AcmeTwilio(t)
	queueStep(t, env, fixtures.AutomationRunJobsSMSStopLinkConfirmedID)
	runSteps(t, env)
	token := unsubscribeLink(t, env)
	env.DB.Contact.UpdateOneID(fixtures.ContactTexterConfirmedID).ClearPhone().ExecX(ctx)
	env.DB.Contact.UpdateOneID(fixtures.ContactTexterID).SetPhone(fixtures.ContactTexterConfirmedPhone).ExecX(ctx)

	err := env.SiteAnonymous(t).SitePublicUnsubscribesPerform(ctx, siteapi.SitePublicUnsubscribesPerformParams{Token: token})

	require.NoError(t, err)
	unsubscribed := env.QueuedMessages(t, systemevent.SmsUnsubscribed)
	require.Len(t, unsubscribed, 1)
	data := unsubscribed[0].Data
	assert.Equal(t, strconv.Itoa(fixtures.ContactTexterID), fmt.Sprint(data["contactId"]), "the number's holder stopped")
	assert.Equal(t, events.UnsubscribedByLink, data["via"])
	assert.NotContains(t, data, "automationId", "the holder received no text of the Automation")
	assert.NotContains(t, data, "broadcastId")
	attributions := env.JobsOf(t, jobs.BroadcastUnsubscribeEventArgs{}.Kind())
	assert.Len(t, attributions, 1)
}
