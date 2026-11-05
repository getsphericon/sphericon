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
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/testhelper"
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
	automationID := smsAutomation(t, env, smsStep(account, "Hi {{ first_name }} & co, your cart <is> waiting"))
	confirmSMS(t, env, fixtures.ContactTexterPhone)
	runID := enrollVia(t, env, automationID, fixtures.ContactTexterID)

	runSteps(t, env)

	sent := env.Twilio.Messages()
	require.Len(t, sent, 1)
	assert.Equal(t, account.Account.SID, sent[0].Account)
	assert.Equal(t, fixtures.ContactTexterPhone, sent[0].To)
	assert.Equal(t, testhelper.TwilioNumber, sent[0].From)
	assert.Equal(t, "Hi Tess & co, your cart <is> waiting", sent[0].Body, "plain text: nothing is escaped")
	assert.Equal(t, account.Integration.HookUrl, sent[0].StatusCallback, "every send asks for its delivery reports at the hook")
	run := env.DB.AutomationRun.GetX(t.Context(), runID)
	assert.Equal(t, automationrun.StatusCompleted, run.Status)
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(t.Context())
	assert.Equal(t, outboundmessage.StatusSent, msg.Status)
	assert.Equal(t, outboundmessage.ChannelSms, msg.Channel)
	assert.Equal(t, fixtures.ContactTexterPhone, msg.Destination)
	assert.Equal(t, sent[0].SID, lo.FromPtr(msg.ProviderMessageID), "Twilio's SID is the provider reference")
	smsSent := env.QueuedCount(t, events.NameSmsSent, map[string]any{
		"contactId": fixtures.ContactTexterID, "automationId": automationID, "destination": fixtures.ContactTexterPhone,
		"integrationId": account.Integration.ID, "outboundMessageId": msg.ID,
	})
	assert.Equal(t, 1, smsSent)
}

func TestAnSMSStepThroughAMessagingServiceSendsFromTheService(t *testing.T) {
	env := testhelper.Setup(t)
	account := env.Twilio.AddAccount()
	service := env.Twilio.AddMessagingService(account)
	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteIntegrationsCreate(t.Context(), &siteapi.CreateIntegrationInput{
		Name:   "Acme service",
		Config: testhelper.SiteTwilio(account.SID, "", account.AuthToken, service),
		Params: siteapi.NewOptJsonObject(testhelper.TwilioAuthToken(account.AuthToken)),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	row, ok := res.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "got %T", res)
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

// nothingToPrepare leaves the fixtures as they are.
func nothingToPrepare(*testing.T, *testhelper.TestEnv) {}

func TestAnSMSStepSkipsAContactItCannotTextAndMovesOn(t *testing.T) {
	cases := map[string]struct {
		contact int64
		prepare func(t *testing.T, env *testhelper.TestEnv)
		reason  string
	}{
		"no phone": {contact: fixtures.ContactBobID, reason: eligibility.ReasonNoDestination, prepare: nothingToPrepare},
		"no Confirmation": {
			contact: fixtures.ContactTexterID, reason: eligibility.ReasonUnconfirmed, prepare: nothingToPrepare,
		},
		"an SMS opt-out": {
			contact: fixtures.ContactTexterID, reason: eligibility.ReasonUnsubscribedEverything,
			prepare: func(t *testing.T, env *testhelper.TestEnv) {
				t.Helper()
				confirmSMS(t, env, fixtures.ContactTexterPhone)
				_, err := env.ExternalScoped(t, "contacts:write").UnsubscribesCreate(t.Context(), &externalapi.CreateUnsubscribeInput{
					Channel: externalapi.NewOptConsentChannel(externalapi.ConsentChannelSMS), Destination: fixtures.ContactTexterPhone,
				})
				require.NoError(t, err)
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			account := env.AcmeTwilio(t)
			automationID := smsAutomation(t, env,
				smsStep(account, "Hi"),
				automationsteps.Step{Type: automationsteps.ApplyTag, Tag: "reminded"},
			)
			tc.prepare(t, env)
			runID := enrollVia(t, env, automationID, tc.contact)

			runSteps(t, env)

			sent := env.Twilio.Messages()
			assert.Empty(t, sent)
			run := env.DB.AutomationRun.GetX(t.Context(), runID)
			assert.Equal(t, automationrun.StatusCompleted, run.Status, "the Enrollment moves on")
			skipped := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(t.Context())
			assert.Equal(t, outboundmessage.StatusSkipped, skipped.Status)
			assert.Equal(t, outboundmessage.ChannelSms, skipped.Channel)
			assert.Equal(t, tc.reason, lo.FromPtr(skipped.Reason))
			smsSent := env.QueuedCount(t, events.NameSmsSent, nil)
			assert.Zero(t, smsSent)
		})
	}
}

func TestAMixedSequenceSendsTheEmailsAndRecordsTheSMSSkipForAContactWithNoPhone(t *testing.T) {
	env := testhelper.Setup(t)
	account := env.AcmeTwilio(t)
	automationID := smsAutomation(t, env,
		automationsteps.Step{Type: automationsteps.Email, Subject: "First", Body: "<p>one</p>"},
		smsStep(account, "Text"),
		automationsteps.Step{Type: automationsteps.Email, Subject: "Last", Body: "<p>two</p>"},
	)
	runID := enrollVia(t, env, automationID, fixtures.ContactBobID)

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
	account := env.AcmeTwilio(t)
	automationID := smsAutomation(t, env, smsStep(account, "Stop: {{ unsubscribe_url }}"))
	confirmSMS(t, env, fixtures.ContactTexterPhone)
	enrollVia(t, env, automationID, fixtures.ContactTexterID)
	runSteps(t, env)
	sent := env.Twilio.Messages()
	require.Len(t, sent, 1)
	link := regexp.MustCompile(`/e/u/(\S+)$`).FindStringSubmatch(sent[0].Body)
	require.Len(t, link, 2, "the text carries the unsubscribe link, appended to nothing")

	out, err := env.SiteAnonymous(t).SitePublicUnsubscribesPerform(ctx, siteapi.SitePublicUnsubscribesPerformParams{Token: link[1]})

	require.NoError(t, err)
	assert.IsType(t, &siteapi.SitePublicUnsubscribesPerformNoContent{}, out)
	optOut := env.DB.Unsubscribe.Query().Where(unsubscribe.DestinationEQ(fixtures.ContactTexterPhone)).OnlyX(ctx)
	assert.Equal(t, unsubscribe.ChannelSms, optOut.Channel)
	assert.Equal(t, eligibility.SourceEverything, optOut.SendingSource)
	assert.Equal(t, unsubscribe.ViaLink, lo.FromPtr(optOut.Via))
	dec, err := eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelSMS, fixtures.ContactTexterPhone, eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.Equal(t, eligibility.ReasonUnsubscribedEverything, dec.Reason, "a Broadcast's text does not go either")
}

// Opening a text's unsubscribe link lands on the confirmation page told the link is a text's, so
// the page speaks of text messages and not of emails.
func TestTheSMSUnsubscribeLinkTellsThePageItIsAText(t *testing.T) {
	env := testhelper.Setup(t)
	account := env.AcmeTwilio(t)
	automationID := smsAutomation(t, env, smsStep(account, "Stop: {{ unsubscribe_url }}"))
	confirmSMS(t, env, fixtures.ContactTexterPhone)
	enrollVia(t, env, automationID, fixtures.ContactTexterID)
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
	automationID := smsAutomation(t, env, smsStep(account, "First"), smsStep(account, "Second"))
	confirmSMS(t, env, fixtures.ContactTexterPhone)
	runID := enrollVia(t, env, automationID, fixtures.ContactTexterID)
	stepper := stepperWith(env, env.OutboundModule(), stepping.WithClock(passingClock()))
	require.True(t, env.RunNextStep(t, stepper))
	first := env.Twilio.Messages()
	require.Len(t, first, 1)

	stopped := env.PostTwilioCallback(t, account.Integration.HookUrl, account.Account.AuthToken, url.Values{
		"MessageSid": {"SM0201"}, "AccountSid": {account.Account.SID}, "From": {fixtures.ContactTexterPhone},
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
