package outboundmessages_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/frequencycap"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/infra/postgres"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The count of the Frequency cap (ADR 0178), observed at the Outbound send seam: every Broadcast's
// and Automation's message takes a place at each Destination it is claimed for, under the lock of
// each Destination a cap holds. No cap is enforced yet.

// Tia's two chats on Lacuna's bot (fixtures/bindings.yml).
const (
	tiaChat      = "77401"
	tiaOtherChat = "77402"
)

// counted is how many places the count of dest on ch holds in the Workspace, over the longest period.
func counted(t *testing.T, env *testhelper.TestEnv, ws int64, ch channels.Channel, dest string) int {
	t.Helper()
	n, err := testhelper.Module[*frequencycap.Module](env).Counted(t.Context(), env.DB.Scoped(ws),
		frequencycap.CountedInput{Channel: ch, Destination: dest, Since: time.Now().Add(-frequencycap.Retention)})
	require.NoError(t, err)
	return n
}

// telegramToTia is the Telegram Broadcast message of key to Tia, who has two chats on Lacuna's bot.
func telegramToTia(t *testing.T, env *testhelper.TestEnv, bot testhelper.TelegramBot, key string) outboundmessages.Request {
	t.Helper()
	tia := env.DB.Contact.GetX(t.Context(), fixtures.ContactLacunaTwoChatsID)
	integrationID := int64(bot.Integration.ID)
	return outboundmessages.Request{
		Ref:           outboundmessages.BroadcastRef{BroadcastID: fixtures.BroadcastLacunaTelegramHelloID},
		Key:           key,
		Contact:       tia,
		Content:       outboundmessages.TelegramContent{Text: "Hello"},
		IntegrationID: &integrationID,
	}
}

func sendToTia(t *testing.T, env *testhelper.TestEnv, req outboundmessages.Request) outboundmessages.Result {
	t.Helper()
	res, err := env.OutboundModule().Send(t.Context(), env.DB.Scoped(fixtures.LacunaID), req)
	require.NoError(t, err)
	return res
}

// Acme has no cap on email: the send counts all the same, and takes no lock of the address.
func TestAMarketingEmailWithNoCapIsCountedWithoutItsDestinationLock(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	alice := env.DB.Contact.GetX(t.Context(), fixtures.ContactAliceID)
	key := postgres.DestinationKey(frequencycap.LockKind, fixtures.AcmeID, string(channels.Email), *alice.Email)
	var res outboundmessages.Result

	err := env.RunBesideKey(t, key, func() error {
		var err error
		res, err = env.OutboundModule().Send(t.Context(), acme(env), marketing(t, env, "count:email", alice.ID))
		return err
	})

	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Sent, res.Outcome)
	assert.Equal(t, 1, counted(t, env, fixtures.AcmeID, channels.Email, *alice.Email))
}

// Acme's SMS cap holds every number: a marketing text waits for its number's lock, and counts once it
// is claimed.
func TestAMarketingTextToACappedNumberWaitsForItsDestinationLock(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	account := connectConfirmedTexter(t, env)
	key := postgres.DestinationKey(frequencycap.LockKind, fixtures.AcmeID, string(channels.SMS), fixtures.ContactTexterPhone)
	var res outboundmessages.Result

	errs := env.QueueBehindKey(t, key, testhelper.Writer{Name: "text", Run: func() error {
		var err error
		res, err = env.OutboundModule().Send(t.Context(), acme(env), textTo(t, env, account, "count:sms"))
		return err
	}})

	require.NoError(t, errs["text"])
	assert.Equal(t, outboundmessages.Sent, res.Outcome)
	assert.Equal(t, 1, counted(t, env, fixtures.AcmeID, channels.SMS, fixtures.ContactTexterPhone))
}

func TestATelegramMessageCountsAtEachBindingAndADroppedBindingLosesItsPlace(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.LacunaTelegram(t)

	first := sendToTia(t, env, telegramToTia(t, env, bot, "count:telegram:1"))
	env.Telegram.FailChat(telegramtest.SendMessage, 77402, telegramtest.ChatNotFound)
	second := sendToTia(t, env, telegramToTia(t, env, bot, "count:telegram:2"))

	require.Equal(t, outboundmessages.Sent, first.Outcome)
	require.Equal(t, outboundmessages.Sent, second.Outcome, "the other chat took it")
	assert.Equal(t, 2, counted(t, env, fixtures.LacunaID, channels.Telegram, tiaChat), "both messages reached the first chat")
	assert.Equal(t, 1, counted(t, env, fixtures.LacunaID, channels.Telegram, tiaOtherChat), "only the first reached the chat that is gone")
}

func TestASendCapDeferralLeavesNoPlaceCounted(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := sendCapModule(env, c)
	capped(t, env, fixtures.AcmeID, new(1))

	require.Equal(t, outboundmessages.Sent, send(t, m, env, "count:cap:1").Outcome)
	res := send(t, m, env, "count:cap:2")

	require.Equal(t, outboundmessages.Deferral, res.Outcome)
	assert.Equal(t, 1, counted(t, env, fixtures.AcmeID, channels.Email, fixtures.ContactAliceEmail), "the deferred message holds no place")
}

func TestAFailedMessageFreesItsPlace(t *testing.T) {
	env := testhelper.Setup(t)

	req := withSubject(marketing(t, env, "count:failed", fixtures.ContactAliceID), "{% if %}broken")
	res, err := newModule(env, time.Now).Send(t.Context(), acme(env), req)

	require.NoError(t, err)
	require.Equal(t, outboundmessages.Failed, res.Outcome)
	assert.Zero(t, counted(t, env, fixtures.AcmeID, channels.Email, fixtures.ContactAliceEmail))
}

func TestASkippedMessageFreesItsPlaces(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.LacunaTelegram(t)
	env.Telegram.FailChat(telegramtest.SendMessage, 77401, telegramtest.ChatNotFound)
	env.Telegram.FailChat(telegramtest.SendMessage, 77402, telegramtest.ChatNotFound)

	res := sendToTia(t, env, telegramToTia(t, env, bot, "count:skipped"))

	require.Equal(t, outboundmessages.Skipped, res.Outcome)
	assert.Zero(t, counted(t, env, fixtures.LacunaID, channels.Telegram, tiaChat))
	assert.Zero(t, counted(t, env, fixtures.LacunaID, channels.Telegram, tiaOtherChat))
}

// A provider failure hands the claim back, and the retry takes it over: the claim held its place all
// along, so the retry counts nothing new.
func TestAClaimTakenOverDoesNotCountTwice(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env, time.Now)

	env.CustomerMail.SetErr(errors.New("smtp unavailable"))
	_, err := m.Send(t.Context(), acme(env), marketing(t, env, "count:retry", fixtures.ContactAliceID))
	require.Error(t, err)
	env.CustomerMail.SetErr(nil)
	res, err := m.Send(t.Context(), acme(env), marketing(t, env, "count:retry", fixtures.ContactAliceID))

	require.NoError(t, err)
	require.Equal(t, outboundmessages.Sent, res.Outcome)
	assert.Equal(t, 1, counted(t, env, fixtures.AcmeID, channels.Email, fixtures.ContactAliceEmail))
}

func TestATransactionalEmailIsNotCounted(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := newModule(env, time.Now).Send(t.Context(), acme(env), transactional("count:transactional", fixtures.ContactAliceEmail))

	require.NoError(t, err)
	require.Equal(t, outboundmessages.Sent, res.Outcome)
	assert.Zero(t, counted(t, env, fixtures.AcmeID, channels.Email, fixtures.ContactAliceEmail))
}
