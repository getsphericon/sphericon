package outboundmessages_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/sending/frequencycap"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/infra/messaging/twilio/twiliotest"
	"github.com/getsphericon/sphericon/internal/infra/postgres"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Frequency cap (ADR 0137, ADR 0178) at the Outbound send seam: a target over its Destination's
// cap is dropped from the message, and a send none of whose targets has a place is Skipped with
// frequency_cap. Every Workspace caps SMS at three a day (fixtures/frequency_caps.yml).

// textTexter sends n marketing texts to ContactTexter's number, each under its own key, and requires
// each Sent: the number's count then holds n places.
func textTexter(t *testing.T, env *testhelper.TestEnv, account testhelper.TwilioAccount, n int) {
	t.Helper()
	for i := range n {
		res := sendText(t, env, textTo(t, env, account, fmt.Sprintf("cap:seed:%d", i)))
		require.Equal(t, outboundmessages.Sent, res.Outcome)
	}
}

func TestAMarketingTextToANumberWithThreeTextsInADayIsSkippedForTheFrequencyCap(t *testing.T) {
	env := testhelper.Setup(t)
	account := connectConfirmedTexter(t, env)
	textTexter(t, env, account, 3)

	res := sendText(t, env, textTo(t, env, account, "cap:fourth"))

	assert.Equal(t, outboundmessages.Skipped, res.Outcome)
	assert.Equal(t, skipreason.FrequencyCap, res.SkippedReason)
	msg := byKey(t, env, "cap:fourth")
	assert.Equal(t, skipreason.FrequencyCap, lo.FromPtr(msg.SkippedReason), "the Skipped is recorded under the key")
	assert.Len(t, env.Twilio.Messages(), 3, "the fourth text is not sent")
	n := counted(t, env, fixtures.AcmeID, channels.SMS, fixtures.ContactTexterPhone)
	assert.Equal(t, 3, n, "a Skipped send takes no place")
}

// A Skipped over the cap is recorded under the Request's key: the same Request replays it.
func TestASendSkippedForTheFrequencyCapReplaysItsOutcome(t *testing.T) {
	env := testhelper.Setup(t)
	account := connectConfirmedTexter(t, env)
	textTexter(t, env, account, 3)
	first := sendText(t, env, textTo(t, env, account, "cap:replay"))

	again := sendText(t, env, textTo(t, env, account, "cap:replay"))

	assert.Equal(t, outboundmessages.Skipped, again.Outcome)
	assert.Equal(t, skipreason.FrequencyCap, again.SkippedReason)
	assert.True(t, again.Replayed)
	assert.Equal(t, first.MessageID, again.MessageID)
}

// Lacuna caps Telegram at one message a day. Tia's other chat is opted out of Broadcasts for the
// first message, so only her first chat takes a place; the second message goes to the other chat
// alone.
func TestATelegramMessageGoesOnlyToTheBindingsUnderTheirCap(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.LacunaTelegram(t)
	env.DB.FrequencyCap.UpdateOneID(fixtures.FrequencyCapLacunaTelegramID).SetMaxMessages(1).SetPeriodMinutes(1440).ExecX(ctx)
	env.DB.Unsubscribe.UpdateOneID(fixtures.UnsubscribeLacunaStoppedID).SetDestination(tiaOtherChat).ExecX(ctx)
	first := sendToTia(t, env, telegramToTia(t, env, bot, "cap:telegram:1"))
	env.DB.Unsubscribe.DeleteOneID(fixtures.UnsubscribeLacunaStoppedID).ExecX(ctx)

	second := sendToTia(t, env, telegramToTia(t, env, bot, "cap:telegram:2"))

	require.Equal(t, outboundmessages.Sent, first.Outcome)
	assert.Equal(t, outboundmessages.Sent, second.Outcome)
	chats := lo.Map(env.Telegram.RequestsOf(telegramtest.SendMessage), func(r telegramtest.Request, _ int) string { return r.String("chat_id") })
	assert.Equal(t, []string{tiaChat, tiaOtherChat}, chats, "the first went to the first chat, the second to the other only")
	dropped := counted(t, env, fixtures.LacunaID, channels.Telegram, tiaChat)
	other := counted(t, env, fixtures.LacunaID, channels.Telegram, tiaOtherChat)
	assert.Equal(t, 1, dropped, "the dropped chat's count does not grow")
	assert.Equal(t, 1, other)
}

// Two Broadcast texts to one number at its last place, queued behind its lock: the first takes the
// place, the second reads it taken and is Skipped.
func TestTwoTextsAtANumbersLastPlaceTakeItOnce(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	account := connectConfirmedTexter(t, env)
	textTexter(t, env, account, 2)
	key := postgres.DestinationKey(frequencycap.LockKind, fixtures.AcmeID, string(channels.SMS), fixtures.ContactTexterPhone)
	var first, second outboundmessages.Result
	writer := func(name string, res *outboundmessages.Result) testhelper.Writer {
		return testhelper.Writer{Name: name, Run: func() error {
			var err error
			*res, err = env.OutboundModule().Send(t.Context(), acme(env), textTo(t, env, account, "cap:race:"+name))
			return err
		}}
	}

	errs := env.QueueBehindKey(t, key, writer("first", &first), writer("second", &second))

	require.NoError(t, errs["first"])
	require.NoError(t, errs["second"])
	assert.Equal(t, outboundmessages.Sent, first.Outcome)
	assert.Equal(t, outboundmessages.Skipped, second.Outcome)
	assert.Equal(t, skipreason.FrequencyCap, second.SkippedReason)
	n := counted(t, env, fixtures.AcmeID, channels.SMS, fixtures.ContactTexterPhone)
	assert.Equal(t, 3, n)
}

// A Test send is no Outbound message: a number at its cap still gets it, and its count does not grow.
func TestATestSendToANumberAtItsCapGoesOut(t *testing.T) {
	env := testhelper.Setup(t)
	account := connectConfirmedTexter(t, env)
	textTexter(t, env, account, 3)
	b := env.DB.Broadcast.GetX(t.Context(), fixtures.BroadcastSmsSentID)

	res, err := env.OutboundModule().SendBroadcastTest(t.Context(), acme(env), b, outboundmessages.TestAddress{Phone: fixtures.ContactTexterPhone})

	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Sent, res.Outcome)
	texted := lo.Map(env.Twilio.Messages(), func(m twiliotest.Message, _ int) string { return m.To })
	assert.Equal(t, []string{fixtures.ContactTexterPhone, fixtures.ContactTexterPhone, fixtures.ContactTexterPhone, fixtures.ContactTexterPhone}, texted)
	n := counted(t, env, fixtures.AcmeID, channels.SMS, fixtures.ContactTexterPhone)
	assert.Equal(t, 3, n)
}

// capEmail gives Acme a cap of most emails a day in place of its caps, as its settings would.
func capEmail(t *testing.T, env *testhelper.TestEnv, most int) {
	t.Helper()
	m := testhelper.Module[*workspaces.Module](env)
	rules, err := m.SendingRules(t.Context(), acme(env))
	require.NoError(t, err)
	_, err = m.ReplaceSendingRules(t.Context(), acme(env), workspaces.ReplaceSendingRulesInput{
		Revision:       rules.Revision,
		FrequencyCaps:  []workspaces.FrequencyCap{{Channel: channels.Email, MaxMessages: &most, PeriodMinutes: new(1440)}},
		SendingWindows: rules.SendingWindows,
	})
	require.NoError(t, err)
}

// Acme caps email at one a day: Alice's address is at its cap after one Broadcast message. A
// Transactional email still goes and takes no place; another Broadcast message is Skipped.
func TestATransactionalEmailIsNeitherCountedNorCapped(t *testing.T) {
	env := testhelper.Setup(t)
	capEmail(t, env, 1)
	m := env.OutboundModule()
	_, err := m.Send(t.Context(), acme(env), marketing(t, env, "cap:email:1", fixtures.ContactAliceID))
	require.NoError(t, err)

	receipt, err := m.Send(t.Context(), acme(env), transactional("cap:email:receipt", fixtures.ContactAliceEmail))
	require.NoError(t, err)
	next, err := m.Send(t.Context(), acme(env), marketing(t, env, "cap:email:2", fixtures.ContactAliceID))
	require.NoError(t, err)

	assert.Equal(t, outboundmessages.Sent, receipt.Outcome)
	assert.Equal(t, outboundmessages.Skipped, next.Outcome)
	assert.Equal(t, skipreason.FrequencyCap, next.SkippedReason)
	n := counted(t, env, fixtures.AcmeID, channels.Email, fixtures.ContactAliceEmail)
	assert.Equal(t, 1, n)
}

// Alice gets three emails an hour apart under a cap of three a day, which is then lowered to two:
// her count holds three places, and a place frees when the second oldest is a day old. The probe
// reads it at any moment and writes nothing.
func TestTheProbeReadsTheCountAndTheReleaseMomentOfADestination(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, c.Now)
	capEmail(t, env, 3)
	start := c.Now()
	for i := range 3 {
		seed := send(t, m, env, fmt.Sprintf("probe:%d", i))
		require.Equal(t, outboundmessages.Sent, seed.Outcome)
		c.Advance(time.Hour)
	}
	capEmail(t, env, 2)
	probe := func(at time.Time) frequencycap.Standing {
		sts, err := testhelper.Module[*frequencycap.Module](env).Probe(t.Context(), acme(env),
			frequencycap.ProbeInput{Channel: channels.Email, Destinations: []string{fixtures.ContactAliceEmail}, At: at})
		require.NoError(t, err)
		return sts[fixtures.ContactAliceEmail]
	}

	now := probe(start.Add(3 * time.Hour))
	released := probe(start.Add(25 * time.Hour))
	before := probe(start.Add(-time.Minute))

	assert.Equal(t, 3, now.Count)
	assert.True(t, now.Over())
	assert.WithinDuration(t, start.Add(25*time.Hour), now.Release, time.Millisecond, "the second oldest leaves the period")
	assert.Equal(t, 1, released.Count, "the two oldest left the period")
	assert.False(t, released.Over())
	assert.True(t, released.Release.IsZero())
	assert.Zero(t, before.Count, "nothing was counted before the first")
	n := counted(t, env, fixtures.AcmeID, channels.Email, fixtures.ContactAliceEmail)
	assert.Equal(t, 3, n, "the probe wrote nothing")
}

// A place counted by a worker whose clock runs ahead still holds for a worker whose clock is behind:
// under the lock every place in the period counts, whatever moment its row carries.
func TestAPlaceCountedByAWorkerAheadHoldsForOneBehind(t *testing.T) {
	env := testhelper.Setup(t)
	behind := startClock()
	capEmail(t, env, 1)
	ahead := testhelper.NewClock(behind.Now().Add(time.Second))
	first := send(t, newModule(env, ahead.Now), env, "cap:ahead")

	second := send(t, newModule(env, behind.Now), env, "cap:behind")

	require.Equal(t, outboundmessages.Sent, first.Outcome)
	assert.Equal(t, outboundmessages.Skipped, second.Outcome)
	assert.Equal(t, skipreason.FrequencyCap, second.SkippedReason)
}

// The claim of the second message counted Tia's other chat only, her first being at its cap. The
// provider fails once and the retry takes the claim over: it goes to the chat its claim counted, not
// to the one the cap dropped.
func TestARetryGoesOnlyToTheDestinationsItsClaimCounted(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.LacunaTelegram(t)
	env.DB.FrequencyCap.UpdateOneID(fixtures.FrequencyCapLacunaTelegramID).SetMaxMessages(1).SetPeriodMinutes(1440).ExecX(ctx)
	env.DB.Unsubscribe.UpdateOneID(fixtures.UnsubscribeLacunaStoppedID).SetDestination(tiaOtherChat).ExecX(ctx)
	seed := sendToTia(t, env, telegramToTia(t, env, bot, "cap:retry:1"))
	require.Equal(t, outboundmessages.Sent, seed.Outcome)
	env.DB.Unsubscribe.DeleteOneID(fixtures.UnsubscribeLacunaStoppedID).ExecX(ctx)
	env.Telegram.FailOnce(telegramtest.SendMessage, telegramtest.Failure{Code: http.StatusInternalServerError, Description: "Internal Server Error"})
	_, err := env.OutboundModule().Send(ctx, env.DB.Scoped(fixtures.LacunaID), telegramToTia(t, env, bot, "cap:retry:2"))
	require.Error(t, err)

	retry := sendToTia(t, env, telegramToTia(t, env, bot, "cap:retry:2"))

	assert.Equal(t, outboundmessages.Sent, retry.Outcome)
	chats := lo.Map(env.Telegram.RequestsOf(telegramtest.SendMessage), func(r telegramtest.Request, _ int) string { return r.String("chat_id") })
	assert.Equal(t, []string{tiaChat, tiaOtherChat, tiaOtherChat}, chats, "the retry does not reach the chat at its cap")
	n := counted(t, env, fixtures.LacunaID, channels.Telegram, tiaChat)
	assert.Equal(t, 1, n)
}

// As above, but Tia's other chat opts out of Broadcasts before the retry: the one chat the claim
// holds a place at is no longer a target, and the chat at its cap is not one the claim may reach, so
// the retry is Skipped.
func TestARetryWhoseCountedDestinationsAreGoneIsSkipped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.LacunaTelegram(t)
	env.DB.FrequencyCap.UpdateOneID(fixtures.FrequencyCapLacunaTelegramID).SetMaxMessages(1).SetPeriodMinutes(1440).ExecX(ctx)
	env.DB.Unsubscribe.UpdateOneID(fixtures.UnsubscribeLacunaStoppedID).SetDestination(tiaOtherChat).ExecX(ctx)
	seed := sendToTia(t, env, telegramToTia(t, env, bot, "cap:gone:1"))
	require.Equal(t, outboundmessages.Sent, seed.Outcome)
	env.DB.Unsubscribe.UpdateOneID(fixtures.UnsubscribeLacunaStoppedID).SetDestination("77407").ExecX(ctx)
	env.Telegram.FailOnce(telegramtest.SendMessage, telegramtest.Failure{Code: http.StatusInternalServerError, Description: "Internal Server Error"})
	_, err := env.OutboundModule().Send(ctx, env.DB.Scoped(fixtures.LacunaID), telegramToTia(t, env, bot, "cap:gone:2"))
	require.Error(t, err)
	env.DB.Unsubscribe.UpdateOneID(fixtures.UnsubscribeLacunaStoppedID).SetDestination(tiaOtherChat).ExecX(ctx)

	retry := sendToTia(t, env, telegramToTia(t, env, bot, "cap:gone:2"))

	assert.Equal(t, outboundmessages.Skipped, retry.Outcome)
	assert.Equal(t, skipreason.NoDestination, retry.SkippedReason)
	chats := lo.Map(env.Telegram.RequestsOf(telegramtest.SendMessage), func(r telegramtest.Request, _ int) string { return r.String("chat_id") })
	assert.Equal(t, []string{tiaChat, tiaOtherChat}, chats, "the retry reaches neither chat")
	n := counted(t, env, fixtures.LacunaID, channels.Telegram, tiaChat)
	assert.Equal(t, 1, n)
}
