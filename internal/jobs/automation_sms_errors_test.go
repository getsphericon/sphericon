package jobs_test

import (
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// sendCalls counts the messages Twilio was asked to create, accepted or not.
func sendCalls(env *testhelper.TestEnv) int {
	return lo.CountBy(env.Twilio.Requests(), func(r twiliotest.Request) bool {
		return strings.HasSuffix(r.Path, "/Messages.json")
	})
}

// A refusal of the account (credentials Twilio no longer accepts, a suspended account) holds the
// Integration: the Enrollment waits at its step, the attention list names the Integration, no
// further send asks Twilio, and saving the Integration with working credentials lets the step send.
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

			assert.Equal(t, automationrun.StatusActive, env.DB.AutomationRun.GetX(ctx, runID).Status)
			assert.Equal(t, []map[string]any{stepJob(runID, 0)}, queuedSteps(t, env), "the held step is asked again later")
			route, err := env.OutboundModule().Route(ctx, env.DB.Scoped(fixtures.AcmeID), "sms", new(int64(account.Integration.ID)))
			require.NoError(t, err)
			assert.Equal(t, outbound.HoldProviderRefused, route.Hold)
			_, named := itemAbout(attentionOf(t, env), siteapi.SiteAttentionKindIntegrationProviderRefused, int64(account.Integration.ID))
			assert.True(t, named, "the attention list names the Integration")
			assert.Zero(t, env.OutboxCount(t, events.NameSmsSent, nil))

			asked := sendCalls(env)
			require.True(t, env.RunNextStep(t, mod))
			assert.Equal(t, asked, sendCalls(env), "while held, the step does not ask Twilio")

			env.Twilio.AcceptSends()
			key := env.Twilio.AddAPIKey(account.Account)
			res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{
				Config: siteapi.NewOptIntegrationConfigInput(testhelper.SiteTwilio(account.Account.SID, key.SID, key.Secret, testhelper.TwilioNumber)),
			}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: account.Integration.ID})
			require.NoError(t, err)
			require.IsType(t, &siteapi.SiteIntegrationResource{}, res)

			env.RunQueuedSteps(t, mod)

			assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status)
			assert.Equal(t, 1, env.OutboxCount(t, events.NameSmsSent, nil))
		})
	}
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

			assert.Equal(t, automationrun.StatusActive, env.DB.AutomationRun.GetX(ctx, runID).Status)
			assert.Zero(t, env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).CountX(ctx), "nothing is recorded")
			jobs := env.JobRowsOf(t, stepping.RunStepArgs{}.Kind())
			require.Len(t, jobs, 1)
			assert.GreaterOrEqual(t, jobs[0].ScheduledAt.Sub(before), outbound.DeferralBackoff)

			env.RunQueuedSteps(t, mod)

			assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status)
			assert.Equal(t, 2, sendCalls(env))
			assert.Equal(t, 1, env.OutboxCount(t, events.NameSmsSent, nil))
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
			env.Twilio.RefuseSends(tc.refusal)

			runSteps(t, env)

			msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(ctx)
			assert.Equal(t, outboundmessage.StatusFailed, msg.Status)
			assert.Equal(t, tc.reason, lo.FromPtr(msg.Reason))
			assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status, "the Enrollment goes on")
			assert.Equal(t, 1, sendCalls(env), "a refusal of the number is not retried")
			assert.Zero(t, env.DB.Suppression.Query().Where(entsuppression.ChannelEQ(entsuppression.ChannelSms)).CountX(ctx))
			assert.Zero(t, env.OutboxCount(t, events.NameSmsSent, nil))
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
	env.Twilio.RefuseSends(twiliotest.NotMobile)

	runSteps(t, env)

	suppressed := env.DB.Suppression.Query().Where(entsuppression.ChannelEQ(entsuppression.ChannelSms)).OnlyX(ctx)
	assert.Equal(t, fixtures.ContactTexterPhone, suppressed.Destination)
	assert.Equal(t, entsuppression.ReasonBounce, suppressed.Reason)
	assert.Equal(t, int64(fixtures.ContactTexterID), lo.FromPtr(suppressed.ContactID))
	msgs := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).
		Order(outboundmessage.ByID()).AllX(ctx)
	require.Len(t, msgs, 2)
	for _, msg := range msgs {
		assert.Equal(t, outboundmessage.StatusSkipped, msg.Status)
		assert.Equal(t, eligibility.ReasonSuppressed, lo.FromPtr(msg.Reason))
	}
	assert.Equal(t, 1, sendCalls(env), "the second text never reached Twilio")
	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status)
}
