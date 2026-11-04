package jobs_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/automationsteps"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging/twilio/twiliotest"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// What Twilio answers at send time is sorted by whom it concerns (ADR 0079): the account, the rate,
// the number, a number that reaches no handset, or nothing anyone can tell. Twilio is the fake REST
// API, scripted to refuse.

// A refusal of the account (credentials Twilio no longer accepts, a suspended account) holds the
// Integration: the Enrollment waits at its step, the attention list names the Integration, and
// saving the Integration with working credentials lets the step send.
func TestATwilioAccountRefusalHoldsTheIntegrationUntilItIsSavedAgain(t *testing.T) {
	cases := map[string]func(t *testing.T, env *testhelper.TestEnv, account testhelper.TwilioAccount){
		"credentials refused": func(_ *testing.T, env *testhelper.TestEnv, account testhelper.TwilioAccount) {
			env.Twilio.RevokeAPIKey(account.Key)
		},
		"account not active": func(_ *testing.T, env *testhelper.TestEnv, _ testhelper.TwilioAccount) {
			env.Twilio.RefuseSends(twiliotest.AccountNotActive)
		},
	}
	for name, refuse := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			account := env.ConnectTwilio(t)
			automationID := smsAutomation(t, env, smsStep(account, "Hello"))
			confirmSMS(t, env, fixtures.ContactTexterPhone)
			runID := enrollVia(t, env, automationID, fixtures.ContactTexterID)
			mod := stepperWith(env, env.OutboundModule(), stepping.WithClock(passingClock()))
			refuse(t, env, account)

			require.True(t, env.RunNextStep(t, mod))

			held := env.DB.AutomationRun.GetX(ctx, runID)
			assert.Equal(t, automationrun.StatusActive, held.Status)
			queued := queuedSteps(t, env)
			assert.Equal(t, []map[string]any{stepJob(runID, 0)}, queued, "the held step is asked again later")
			route, err := env.OutboundModule().Route(ctx, env.DB.Scoped(fixtures.AcmeID), "sms", new(int64(account.Integration.ID)))
			require.NoError(t, err)
			assert.Equal(t, outbound.HoldProviderRefused, route.Hold)
			attention := attentionOf(t, env)
			_, named := itemAbout(attention, siteapi.SiteAttentionKindIntegrationProviderRefused, int64(account.Integration.ID))
			assert.True(t, named, "the attention list names the Integration")
			sentWhileHeld := env.OutboxCount(t, events.NameSmsSent, nil)
			assert.Zero(t, sentWhileHeld)

			env.Twilio.AcceptSends()
			saveTwilioKey(t, env, account, env.Twilio.AddAPIKey(account.Account))
			env.RunQueuedSteps(t, mod)

			done := env.DB.AutomationRun.GetX(ctx, runID)
			assert.Equal(t, automationrun.StatusCompleted, done.Status)
			sent := env.OutboxCount(t, events.NameSmsSent, nil)
			assert.Equal(t, 1, sent)
		})
	}
}

// While the Integration is held, its steps do not ask Twilio even once Twilio would accept again:
// only saving the Integration lifts the hold, with the same credentials once the account works again.
func TestAHeldTwilioIntegrationSendsNothingUntilItIsSavedAgain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.ConnectTwilio(t)
	automationID := smsAutomation(t, env, smsStep(account, "Hello"))
	confirmSMS(t, env, fixtures.ContactTexterPhone)
	runID := enrollVia(t, env, automationID, fixtures.ContactTexterID)
	mod := stepperWith(env, env.OutboundModule(), stepping.WithClock(passingClock()))
	env.Twilio.RefuseSends(twiliotest.AccountNotActive)
	require.True(t, env.RunNextStep(t, mod))
	env.Twilio.AcceptSends()

	require.True(t, env.RunNextStep(t, mod))

	inbox := env.Twilio.Messages()
	assert.Empty(t, inbox, "while held, the step does not ask Twilio")
	held := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, automationrun.StatusActive, held.Status)

	saveTwilioKey(t, env, account, account.Key)
	env.RunQueuedSteps(t, mod)

	delivered := env.Twilio.Messages()
	assert.Equal(t, []string{fixtures.ContactTexterPhone}, recipientsOf(delivered))
}

// saveTwilioKey saves the Integration with key, as an Owner fixing its credentials does.
func saveTwilioKey(t *testing.T, env *testhelper.TestEnv, account testhelper.TwilioAccount, key twiliotest.APIKey) {
	t.Helper()
	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteIntegrationsUpdate(t.Context(), &siteapi.UpdateIntegrationInput{
		Config: siteapi.NewOptIntegrationConfigInput(testhelper.SiteTwilio(account.Account.SID, key.SID, key.Secret, testhelper.TwilioNumber)),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: account.Integration.ID})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteIntegrationResource{}, res)
}

// recipientsOf is the numbers the texts Twilio accepted went to, in order: the recipients' inbox.
func recipientsOf(inbox []twiliotest.Message) []string {
	return lo.Map(inbox, func(m twiliotest.Message, _ int) string { return m.To })
}

// attentionOf is what needs attention in Acme, as its Owner reads it.
func attentionOf(t *testing.T, env *testhelper.TestEnv) []siteapi.SiteAttentionItem {
	t.Helper()
	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteAttentionGet(t.Context(), siteapi.SiteAttentionGetParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	list, ok := res.(*siteapi.SiteAttentionList)
	require.Truef(t, ok, "got %T", res)
	return list.Items
}

// itemAbout is the item of the kind about the subject id, and whether there is one.
func itemAbout(items []siteapi.SiteAttentionItem, kind siteapi.SiteAttentionKind, id int64) (siteapi.SiteAttentionItem, bool) {
	return lo.Find(items, func(i siteapi.SiteAttentionItem) bool {
		return i.Kind == kind && i.Subject.Value.ID == siteapi.EntityId(id)
	})
}

// A rate signal (a 429, a full queue) defers the send: the message is not failed, the step is asked
// again after the backoff and then sends.
func TestATwilioRateSignalDefersTheStepAndItSendsLater(t *testing.T) {
	for name, refusal := range map[string]twiliotest.Refusal{
		"too many requests": twiliotest.TooManyRequests, "queue full": twiliotest.QueueFull,
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			account := env.ConnectTwilio(t)
			automationID := smsAutomation(t, env, smsStep(account, "Hello"))
			confirmSMS(t, env, fixtures.ContactTexterPhone)
			runID := enrollVia(t, env, automationID, fixtures.ContactTexterID)
			mod := stepperWith(env, env.OutboundModule())
			env.Twilio.RefuseNextSend(refusal)
			before := time.Now()

			require.True(t, env.RunNextStep(t, mod))

			deferred := env.DB.AutomationRun.GetX(ctx, runID)
			assert.Equal(t, automationrun.StatusActive, deferred.Status)
			recorded := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).CountX(ctx)
			assert.Zero(t, recorded, "nothing is recorded")
			jobs := env.JobRowsOf(t, stepping.RunStepArgs{}.Kind())
			require.Len(t, jobs, 1)
			assert.GreaterOrEqual(t, jobs[0].ScheduledAt.Sub(before), outbound.DeferralBackoff)

			env.RunQueuedSteps(t, mod)

			done := env.DB.AutomationRun.GetX(ctx, runID)
			assert.Equal(t, automationrun.StatusCompleted, done.Status)
			inbox := env.Twilio.Messages()
			assert.Equal(t, []string{fixtures.ContactTexterPhone}, recipientsOf(inbox), "the text went once, after the backoff")
			sent := env.OutboxCount(t, events.NameSmsSent, nil)
			assert.Equal(t, 1, sent)
		})
	}
}

// A refusal of the number, or one that tells nothing of whom it concerns, fails that message with
// Twilio's words and code, suppresses nothing, and the Enrollment goes on to its next step.
func TestATwilioRefusalOfTheNumberFailsThatMessageAndTheRunGoesOn(t *testing.T) {
	cases := map[string]struct {
		refusal twiliotest.Refusal
		reason  string
	}{
		"invalid number":  {twiliotest.InvalidTo, "message_refused: Invalid 'To' Phone Number (error 21211)"},
		"an unknown code": {twiliotest.Unknown, "message_refused: Something Twilio does not explain (error 29999)"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			account := env.ConnectTwilio(t)
			automationID := smsAutomation(t, env,
				smsStep(account, "Hello"),
				automationsteps.Step{Type: automationsteps.ApplyTag, Tag: "texted"},
			)
			confirmSMS(t, env, fixtures.ContactTexterPhone)
			runID := enrollVia(t, env, automationID, fixtures.ContactTexterID)
			env.Twilio.RefuseNextSend(tc.refusal)

			runSteps(t, env)

			msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(ctx)
			assert.Equal(t, outboundmessage.StatusFailed, msg.Status)
			assert.Equal(t, tc.reason, lo.FromPtr(msg.Reason))
			run := env.DB.AutomationRun.GetX(ctx, runID)
			assert.Equal(t, automationrun.StatusCompleted, run.Status, "the Enrollment goes on")
			inbox := env.Twilio.Messages()
			assert.Empty(t, inbox, "a refusal of the number is not retried, though Twilio would accept it now")
			suppressed := env.DB.Suppression.Query().Where(entsuppression.ChannelEQ(entsuppression.ChannelSms)).CountX(ctx)
			assert.Zero(t, suppressed)
			sent := env.OutboxCount(t, events.NameSmsSent, nil)
			assert.Zero(t, sent)
		})
	}
}

// A number Twilio says is no mobile number is suppressed on sms like a hard bounce: the message is
// Skipped as suppressed, the Enrollment goes on, and the next text to the number is skipped without
// asking Twilio.
func TestANumberTwilioSaysIsNoMobileNumberIsSuppressed(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.ConnectTwilio(t)
	automationID := smsAutomation(t, env, smsStep(account, "Hello"), smsStep(account, "Again"))
	confirmSMS(t, env, fixtures.ContactTexterPhone)
	runID := enrollVia(t, env, automationID, fixtures.ContactTexterID)
	env.Twilio.RefuseNextSend(twiliotest.NotMobile)

	runSteps(t, env)

	suppressed := env.DB.Suppression.Query().Where(entsuppression.ChannelEQ(entsuppression.ChannelSms)).OnlyX(ctx)
	assert.Equal(t, fixtures.ContactTexterPhone, suppressed.Destination)
	assert.Equal(t, entsuppression.ReasonBounce, suppressed.Reason)
	assert.Equal(t, int64(fixtures.ContactTexterID), lo.FromPtr(suppressed.ContactID))
	msgs := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).
		Order(outboundmessage.ByID()).AllX(ctx)
	skipped := lo.Map(msgs, func(m *ent.OutboundMessage, _ int) string { return string(m.Status) + ": " + lo.FromPtr(m.Reason) })
	suppressedSkip := string(outboundmessage.StatusSkipped) + ": " + eligibility.ReasonSuppressed
	assert.Equal(t, []string{suppressedSkip, suppressedSkip}, skipped)
	inbox := env.Twilio.Messages()
	assert.Empty(t, inbox, "the second text never reached Twilio, though Twilio would accept it")
	run := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, automationrun.StatusCompleted, run.Status)
}
