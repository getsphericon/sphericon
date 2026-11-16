package jobs_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Telegram consent observations apply in the order they occurred (ADR 0154): within one bot by
// update_id, between bots by the update's date, and of the same moment the refusal wins.

// saidAt is said at date (Unix seconds).
func saidAt(updateID, from, date int64, text string) string {
	return fmt.Sprintf(`{"update_id":%d,"message":{"message_id":%d,"date":%d,"chat":{"id":%d,"type":"private"},"from":{"id":%d,"is_bot":false,"first_name":"P"},"text":%q}}`,
		updateID, updateID, date, from, from, text)
}

func post(t *testing.T, env *testhelper.TestEnv, bot testhelper.TelegramBot, update string) {
	t.Helper()
	reply := env.PostTelegramUpdate(t, bot, update)
	require.Equal(t, http.StatusOK, reply.Code, reply.Body.String())
}

func TestAnOlderUpdateOfTheSameBotChangesNothing(t *testing.T) {
	const date = 1767225600
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	user := mustInt(t, fixtures.BindingLiamDestination)
	post(t, env, bot, saidAt(101, user, date, "/stop"))
	// Within one bot the update_id orders, whatever the dates say.
	post(t, env, bot, saidAt(100, user, date+60, "/start"))
	stopped := optedOut(t, env, user)
	superseded := env.DB.Event.Query().Where(
		event.ContactID(fixtures.ContactLiamID), event.Action(systemevent.ContactConsentSuperseded),
	).AllX(t.Context())
	assert.True(t, stopped, "update 100 came before the stop in 101")
	require.Len(t, superseded, 1, "the start is in the history, as changing nothing")
	assert.Equal(t, "start", superseded[0].Properties["observation"])
}

// telegramUpdate is an update posted to one of Acme's two bots.
type telegramUpdate struct {
	secondBot bool
	body      string
}

func TestAStopAndAStartOfTheSameMomentOnTwoBotsLeaveTheUserStopped(t *testing.T) {
	const date = 1767225600
	user := mustInt(t, fixtures.BindingLiamDestination)
	stop := telegramUpdate{body: saidAt(500, user, date, "/stop")}
	start := telegramUpdate{secondBot: true, body: saidAt(7, user, date, "/start")}
	for name, order := range map[string][]telegramUpdate{"stop first": {stop, start}, "start first": {start, stop}} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			bots := map[bool]testhelper.TelegramBot{false: env.AcmeTelegram(t), true: env.AcmeSecondTelegram(t)}
			for _, u := range order {
				post(t, env, bots[u.secondBot], u.body)
			}
			stopped := optedOut(t, env, user)
			assert.True(t, stopped, "a tie between two bots goes to the refusal")
		})
	}
}

func TestALaterStartOnAnotherBotLiftsTheStop(t *testing.T) {
	const date = 1767225600
	env := testhelper.Setup(t)
	first, second := env.AcmeTelegram(t), env.AcmeSecondTelegram(t)
	user := mustInt(t, fixtures.BindingLiamDestination)
	post(t, env, first, saidAt(500, user, date, "/stop"))
	post(t, env, second, saidAt(7, user, date+1, "/start"))
	stopped := optedOut(t, env, user)
	assert.False(t, stopped)
}
