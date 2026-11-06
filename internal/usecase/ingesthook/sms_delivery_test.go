package ingesthook_test

import (
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging/twilio/twiliotest"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Twilio's delivery reports reach the message they name (ADR 0079): a delivered report stamps it and
// records sms.delivered, an undelivered or failed one stamps it with Twilio's error code and records
// sms.undelivered. A message moves only forward: its first final report stays, and a report of
// another message, or of a status on the way, changes nothing.

// sentText is the text to ContactTexter Twilio accepted through Acme's account (OutboundTexterSms),
// as Twilio received it, and its Outbound message.
func sentText(t *testing.T, env *testhelper.TestEnv) (testhelper.TwilioAccount, twiliotest.Message, *ent.OutboundMessage) {
	t.Helper()
	account := env.AcmeTwilio(t)
	msg := env.DB.OutboundMessage.GetX(t.Context(), fixtures.OutboundTexterSmsID)
	text := twiliotest.Message{
		Account: account.Account.SID, SID: lo.FromPtr(msg.ProviderMessageID), To: msg.Destination,
		From: testhelper.TwilioNumber, StatusCallback: account.Integration.HookUrl,
	}
	return account, text, msg
}

// report posts a status callback of the message and requires the hook to take it.
func report(t *testing.T, env *testhelper.TestEnv, account testhelper.TwilioAccount, msg twiliotest.Message, status, code string) {
	t.Helper()
	res := env.ReportTwilioStatus(t, account, msg, status, code)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
}

func TestADeliveredReportStampsTheTextAndRecordsSmsDelivered(t *testing.T) {
	env := testhelper.Setup(t)
	account, text, msg := sentText(t, env)

	report(t, env, account, text, "delivered", "")
	report(t, env, account, text, "delivered", "")

	got := env.DB.OutboundMessage.GetX(t.Context(), msg.ID)
	assert.NotNil(t, got.DeliveredAt)
	assert.Nil(t, got.UndeliveredAt)
	delivered := env.QueuedCount(t, events.NameSmsDelivered, map[string]any{
		"contactId": fixtures.ContactTexterID, "outboundMessageId": msg.ID, "integrationId": account.Integration.ID,
		"destination": fixtures.ContactTexterPhone,
	})
	assert.Equal(t, 1, delivered, "a redelivered report records nothing more")
	undelivered := env.QueuedCount(t, events.NameSmsUndelivered, nil)
	assert.Zero(t, undelivered)
}

func TestAnUndeliveredOrFailedReportStampsTheTextWithTheErrorCodeAndRecordsSmsUndelivered(t *testing.T) {
	for _, status := range []string{"undelivered", "failed"} {
		t.Run(status, func(t *testing.T) {
			env := testhelper.Setup(t)
			account, text, msg := sentText(t, env)

			report(t, env, account, text, status, "30003")

			got := env.DB.OutboundMessage.GetX(t.Context(), msg.ID)
			assert.NotNil(t, got.UndeliveredAt)
			assert.Equal(t, "30003", lo.FromPtr(got.UndeliveredCode))
			assert.Nil(t, got.DeliveredAt)
			assert.Equal(t, outboundmessage.StatusSent, got.Status, "Twilio accepted it: it stays Sent")
			undelivered := env.QueuedCount(t, events.NameSmsUndelivered, map[string]any{
				"contactId": fixtures.ContactTexterID, "outboundMessageId": msg.ID, "errorCode": "30003",
			})
			assert.Equal(t, 1, undelivered)
			delivered := env.QueuedCount(t, events.NameSmsDelivered, nil)
			assert.Zero(t, delivered)
			suppressions := env.DB.Suppression.Query().Where(entsuppression.WorkspaceID(fixtures.AcmeID), entsuppression.ChannelEQ(channels.SMS)).CountX(t.Context())
			assert.Zero(t, suppressions, "an unreachable handset is no dead number")
		})
	}
}

// Reports arrive out of order: a status on the way after the final one, or the other final one
// after it, changes neither the stamps nor the Events.
func TestAReportNeverMovesATextBackwards(t *testing.T) {
	delivered := finalState{Delivered: true}
	undelivered := finalState{Undelivered: true, Code: "30008"}
	cases := map[string]struct {
		first, firstCode, then, thenCode string
		want                             finalState
	}{
		"sent after delivered":        {first: "delivered", then: "sent", want: delivered},
		"undelivered after delivered": {first: "delivered", then: "undelivered", thenCode: "30003", want: delivered},
		"delivered after undelivered": {first: "undelivered", firstCode: "30008", then: "delivered", want: undelivered},
		"sending after undelivered":   {first: "undelivered", firstCode: "30008", then: "sending", want: undelivered},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			account, text, msg := sentText(t, env)
			report(t, env, account, text, c.first, c.firstCode)

			report(t, env, account, text, c.then, c.thenCode)

			got := env.DB.OutboundMessage.GetX(t.Context(), msg.ID)
			state := finalState{Delivered: got.DeliveredAt != nil, Undelivered: got.UndeliveredAt != nil, Code: lo.FromPtr(got.UndeliveredCode)}
			assert.Equal(t, c.want, state)
			reports := env.QueuedEnvelopes(t, events.NameSmsDelivered, events.NameSmsUndelivered)
			assert.Len(t, reports, 1)
		})
	}
}

// finalState is where a text's delivery reports left it: delivered, or undelivered with a code.
type finalState struct {
	Delivered, Undelivered bool
	Code                   string
}

func TestAReportOfAnUnknownTextChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	account, text, msg := sentText(t, env)
	stranger := text
	stranger.SID = "SM00000000000000000000000000000000"

	report(t, env, account, stranger, "delivered", "")
	report(t, env, account, stranger, "undelivered", "30005")

	got := env.DB.OutboundMessage.GetX(t.Context(), msg.ID)
	assert.Nil(t, got.DeliveredAt)
	assert.Nil(t, got.UndeliveredAt)
	reports := env.QueuedEnvelopes(t, events.NameSmsDelivered, events.NameSmsUndelivered)
	assert.Empty(t, reports)
	suppressions := env.DB.Suppression.Query().Where(entsuppression.WorkspaceID(fixtures.AcmeID), entsuppression.ChannelEQ(channels.SMS)).CountX(t.Context())
	assert.Zero(t, suppressions)
}

// An undelivered report saying the number reaches no handset (Twilio's 30005, 30006) suppresses the
// number like a hard bounce, as the same answer at send time does.
func TestAnUndeliveredReportOfADeadNumberSuppressesIt(t *testing.T) {
	for _, code := range []string{"30005", "30006"} {
		t.Run(code, func(t *testing.T) {
			env := testhelper.Setup(t)
			account, text, _ := sentText(t, env)

			report(t, env, account, text, "undelivered", code)

			suppressed := env.DB.Suppression.Query().Where(entsuppression.WorkspaceID(fixtures.AcmeID), entsuppression.ChannelEQ(channels.SMS)).OnlyX(t.Context())
			assert.Equal(t, fixtures.ContactTexterPhone, suppressed.Destination)
			assert.Equal(t, entsuppression.ReasonBounce, suppressed.Reason)
			assert.Equal(t, int64(fixtures.ContactTexterID), lo.FromPtr(suppressed.ContactID))
			undelivered := env.QueuedCount(t, events.NameSmsUndelivered, map[string]any{"errorCode": code})
			assert.Equal(t, 1, undelivered)
		})
	}
}
