package jobs_test

import (
	"net/http"
	"net/url"
	"regexp"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/automationsteps"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/stepexecution"
)

// The SMS send step (ADR 0081): a Contact whose phone has a Confirmation on sms gets the rendered
// text through the Twilio account the step names; a Contact with no phone, no Confirmation or an
// SMS opt-out is skipped with its reason and the sequence goes on. Twilio is the fake REST API.

// smsTrigger is the customer action the SMS Automations of these tests start on.
const smsTrigger = "cart_abandoned"

// smsAutomation creates and publishes an Automation triggered on smsTrigger that runs steps, and
// returns its id.
func smsAutomation(t *testing.T, env *testhelper.TestEnv, steps ...automationsteps.Step) int64 {
	t.Helper()
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	created, err := m.Create(ctx, s, automations.CreateInput{
		Name: "Cart reminder", TriggerEvent: smsTrigger, ExitEvents: []string{}, Steps: steps,
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, created.ID)
	require.NoError(t, err)
	return created.ID
}

// smsStep is an SMS step through the connected account.
func smsStep(account testhelper.TwilioAccount, text string) automationsteps.Step {
	id := int64(account.Integration.ID)
	return automationsteps.Step{Type: automationsteps.SMS, IntegrationID: &id, Text: text}
}

// confirmSMS records the Confirmation an API client asserts for the number.
func confirmSMS(t *testing.T, env *testhelper.TestEnv, number string) {
	t.Helper()
	_, err := env.ExternalScoped(t, "contacts:write").ConfirmationsCreate(t.Context(), &externalapi.CreateConfirmationInput{
		Channel: externalapi.ConsentChannelSMS, Destination: number,
		Wording: "Yes, text me offers from Acme. Reply STOP to opt out.", Source: "checkout form", Brand: "Acme",
	})
	require.NoError(t, err)
}

func TestAnSMSStepSendsTheRenderedTextToTheConfirmedPhone(t *testing.T) {
	env := testhelper.Setup(t)
	account := env.AcmeTwilio(t)
	automationID := int64(fixtures.AutomationJobsSMSRenderedID)
	runID := int64(fixtures.AutomationRunJobsSMSRenderedConfirmedID)
	queueStep(t, env, runID)

	runSteps(t, env)

	sent := env.Twilio.Messages()
	require.Len(t, sent, 1)
	assert.Equal(t, account.Account.SID, sent[0].Account)
	assert.Equal(t, fixtures.ContactTexterConfirmedPhone, sent[0].To)
	assert.Equal(t, testhelper.TwilioNumber, sent[0].From)
	assert.Equal(t, "Hi Tessa & co, your cart <is> waiting", sent[0].Body, "plain text: nothing is escaped")
	assert.Equal(t, account.Integration.HookUrl, sent[0].StatusCallback, "every send asks for its delivery reports at the hook")
	run := env.DB.AutomationRun.GetX(t.Context(), runID)
	assert.Equal(t, automationrun.StatusCompleted, run.Status)
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(t.Context())
	assert.Equal(t, outboundmessage.StatusSent, msg.Status)
	assert.Equal(t, channels.SMS, msg.Channel)
	assert.Equal(t, fixtures.ContactTexterConfirmedPhone, msg.Destination)
	assert.Equal(t, sent[0].SID, lo.FromPtr(msg.ProviderMessageID), "Twilio's SID is the provider reference")
	smsSent := env.QueuedCount(t, events.NameSmsSent, map[string]any{
		"contactId": fixtures.ContactTexterConfirmedID, "automationId": automationID, "destination": fixtures.ContactTexterConfirmedPhone,
		"integrationId": account.Integration.ID, "outboundMessageId": msg.ID,
	})
	assert.Equal(t, 1, smsSent)
}

func TestAnSMSStepThroughAMessagingServiceSendsFromTheService(t *testing.T) {
	env := testhelper.Setup(t)
	account := env.Twilio.AddAccount()
	service := env.Twilio.AddMessagingService(account)
	row, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteIntegrationsCreate(t.Context(), &siteapi.CreateIntegrationInput{
		Name:   "Acme service",
		Config: testhelper.SiteTwilio(account.SID, "", account.AuthToken, service),
		Params: siteapi.NewOptJsonObject(testhelper.TwilioAuthToken(account.AuthToken)),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	automationID := smsAutomation(t, env, smsStep(testhelper.TwilioAccount{Integration: row}, "Hello"))
	confirmSMS(t, env, fixtures.ContactTexterPhone)
	enrollVia(t, env, automationID, fixtures.ContactTexterID)

	runSteps(t, env)

	sent := env.Twilio.Messages()
	require.Len(t, sent, 1)
	assert.Equal(t, service, sent[0].MessagingServiceSID)
	assert.Empty(t, sent[0].From)
	assert.Equal(t, row.HookUrl, sent[0].StatusCallback)
}

// An SMS step then a tag step (AutomationJobsSMSHiThenTag): a Contact with no phone (Bob), one whose
// phone has no Confirmation (ContactTexter) and one that opted out of every SMS
// (ContactTexterOptedOut) each have an Enrollment there.
func TestAnSMSStepSkipsAContactItCannotTextAndMovesOn(t *testing.T) {
	cases := map[string]struct {
		run    int64
		reason string
	}{
		"no phone":        {run: fixtures.AutomationRunJobsSMSHiThenTagBobID, reason: eligibility.ReasonNoDestination},
		"no Confirmation": {run: fixtures.AutomationRunJobsSMSHiThenTagTexterID, reason: eligibility.ReasonUnconfirmed},
		"an SMS opt-out":  {run: fixtures.AutomationRunJobsSMSHiThenTagOptedOutID, reason: eligibility.ReasonUnsubscribedEverything},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			env.AcmeTwilio(t)
			runID := tc.run
			queueStep(t, env, runID)

			runSteps(t, env)

			sent := env.Twilio.Messages()
			assert.Empty(t, sent)
			run := env.DB.AutomationRun.GetX(t.Context(), runID)
			assert.Equal(t, automationrun.StatusCompleted, run.Status, "the Enrollment moves on")
			skipped := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(t.Context())
			assert.Equal(t, outboundmessage.StatusSkipped, skipped.Status)
			assert.Equal(t, channels.SMS, skipped.Channel)
			assert.Equal(t, tc.reason, lo.FromPtr(skipped.Reason))
			smsSent := env.QueuedCount(t, events.NameSmsSent, nil)
			assert.Zero(t, smsSent)
		})
	}
}

func TestAMixedSequenceSendsTheEmailsAndRecordsTheSMSSkipForAContactWithNoPhone(t *testing.T) {
	env := testhelper.Setup(t)
	env.AcmeTwilio(t)
	runID := int64(fixtures.AutomationRunJobsSMSBetweenEmailsBobID)
	queueStep(t, env, runID)

	runSteps(t, env)

	mails := env.CustomerMail.Messages()
	subjects := lo.Map(mails, func(m messaging.EmailMessage, _ int) string { return m.Subject })
	assert.Equal(t, []string{"First", "Last"}, subjects)
	texts := env.Twilio.Messages()
	assert.Empty(t, texts)
	run := env.DB.AutomationRun.GetX(t.Context(), runID)
	assert.Equal(t, automationrun.StatusCompleted, run.Status)
	skipped := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID), outboundmessage.StatusEQ(outboundmessage.StatusSkipped)).OnlyX(t.Context())
	assert.Equal(t, eligibility.ReasonNoDestination, lo.FromPtr(skipped.Reason))
	assert.Equal(t, 1, lo.FromPtr(skipped.AutomationStep))
}

// The unsubscribe link of a text is the number's Channel-wide opt-out (ADR 0077): a person who
// stops one Automation's texts stops every SMS of the Workspace, as a STOP does.
func TestTheSMSUnsubscribeLinkOptsTheNumberOutOfEverySMS(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.AcmeTwilio(t)
	queueStep(t, env, fixtures.AutomationRunJobsSMSStopLinkConfirmedID)
	runSteps(t, env)
	sent := env.Twilio.Messages()
	require.Len(t, sent, 1)
	link := regexp.MustCompile(`/e/u/(\S+)$`).FindStringSubmatch(sent[0].Body)
	require.Len(t, link, 2, "the text carries the unsubscribe link, appended to nothing")

	err := env.SiteAnonymous(t).SitePublicUnsubscribesPerform(ctx, siteapi.SitePublicUnsubscribesPerformParams{Token: link[1]})

	require.NoError(t, err)
	// The number's opt-out of SMS Broadcasts (UnsubscribeTexterConfirmedBroadcasts) stays beside it.
	optOut := env.DB.Unsubscribe.Query().Where(unsubscribe.DestinationEQ(fixtures.ContactTexterConfirmedPhone),
		unsubscribe.SendingSource(eligibility.SourceEverything)).OnlyX(ctx)
	assert.Equal(t, channels.SMS, optOut.Channel)
	assert.Equal(t, unsubscribe.ViaLink, lo.FromPtr(optOut.Via))
	dec, err := eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelSMS, fixtures.ContactTexterConfirmedPhone, eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.Equal(t, eligibility.ReasonUnsubscribedEverything, dec.Reason, "a Broadcast's text does not go either")
}

// Opening a text's unsubscribe link lands on the confirmation page told the link is a text's, so
// the page speaks of text messages and not of emails.
func TestTheSMSUnsubscribeLinkTellsThePageItIsAText(t *testing.T) {
	env := testhelper.Setup(t)
	env.AcmeTwilio(t)
	queueStep(t, env, fixtures.AutomationRunJobsSMSStopLinkConfirmedID)
	runSteps(t, env)
	sent := env.Twilio.Messages()
	require.Len(t, sent, 1)
	link := regexp.MustCompile(`/e/u/\S+$`).FindString(sent[0].Body)
	require.NotEmpty(t, link)

	opened := follow(t, env, link, browserUA)

	require.Equal(t, http.StatusSeeOther, opened.Code)
	landing, err := url.Parse(opened.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/unsubscribe", landing.Path)
	assert.Equal(t, "sms", landing.Query().Get("channel"))
}

// Send-eligibility is read when each text goes (ADR 0077): a STOP that arrives after an
// Enrollment's first text is honoured for its second.
func TestASTOPBetweenTwoTextsOfAnEnrollmentSkipsTheSecond(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.AcmeTwilio(t)
	runID := int64(fixtures.AutomationRunJobsSMSFirstSecondConfirmedID)
	queueStep(t, env, runID)
	stepper := stepperWith(env, env.OutboundModule(), stepexecution.WithClock(passingClock()))
	require.True(t, env.RunNextStep(t, stepper))
	first := env.Twilio.Messages()
	require.Len(t, first, 1)

	stopped := env.PostTwilioCallback(t, account.Integration.HookUrl, account.Account.AuthToken, url.Values{
		"MessageSid": {"SM0201"}, "AccountSid": {account.Account.SID}, "From": {fixtures.ContactTexterConfirmedPhone},
		"To": {testhelper.TwilioNumber}, "Body": {"STOP"}, "NumMedia": {"0"}, "SmsStatus": {"received"},
	})
	require.Equal(t, http.StatusOK, stopped.Code, stopped.Body.String())
	env.RunQueuedSteps(t, stepper)

	sent := env.Twilio.Messages()
	require.Len(t, sent, 1, "only the text before the STOP went out")
	assert.Equal(t, "First", sent[0].Body)
	run := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, automationrun.StatusCompleted, run.Status)
	second := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID), outboundmessage.StatusEQ(outboundmessage.StatusSkipped)).OnlyX(ctx)
	assert.Equal(t, 1, lo.FromPtr(second.AutomationStep))
	assert.Equal(t, eligibility.ReasonUnsubscribedEverything, lo.FromPtr(second.Reason))
}
