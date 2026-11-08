package jobs_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestMalformedTelegramIdentityCannotRedeemATokenOrResume(t *testing.T) {
	for _, identity := range []string{"", `"update_id":null,`, `"update_id":"1001",`, `"update_id":-1,`, `"update_id":1.5,`, `"update_id":true,`, `"update_id":{},`, `"update_id":[],`, `"update_id":9223372036854775808,`} {
		t.Run(identity, func(t *testing.T) {
			env := testhelper.Setup(t)
			bot := env.AcmeTelegram(t)
			stop := env.PostTelegramUpdate(t, bot, said(1000, optOutUser, "/stop"))
			require.Equal(t, http.StatusOK, stop.Code)
			update := said(1001, optOutUser, "/start "+fixtures.StartTokenAcmeOpenToken)
			update = strings.Replace(update, `"update_id":1001,`, identity, 1)

			reply := env.PostTelegramUpdate(t, bot, update)

			require.Equal(t, http.StatusBadRequest, reply.Code)
			stopped := optedOut(t, env, optOutUser)
			assert.True(t, stopped)
			token := env.DB.StartToken.GetX(t.Context(), fixtures.StartTokenAcmeOpenID)
			assert.Nil(t, token.UsedAt)
			bound := env.DB.Binding.Query().Where(binding.Destination("4101")).ExistX(t.Context())
			assert.False(t, bound)
			resumes := env.QueuedEnvelopes(t, events.NameTelegramResumeObserved)
			assert.Empty(t, resumes)
		})
	}
}

// The Bot API HTTP boundary is the only observable output of the nondurable acknowledgement.
func TestMalformedTelegramIdentityCannotStopOrAcknowledge(t *testing.T) {
	for name, update := range map[string]string{
		"text":   said(1001, optOutUser, "/stop"),
		"button": pressed(1001, optOutUser, 77, "cb-invalid", "unsubscribe"),
		"block":  memberStatus(1001, optOutUser, "kicked"),
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			bot := env.AcmeTelegram(t)
			update = strings.Replace(update, `"update_id":1001,`, `"update_id":null,`, 1)

			reply := env.PostTelegramUpdate(t, bot, update)

			require.Equal(t, http.StatusBadRequest, reply.Code)
			stopped := optedOut(t, env, optOutUser)
			assert.False(t, stopped)
			stops := env.QueuedEnvelopes(t, events.NameTelegramStopObserved)
			assert.Empty(t, stops)
			answers := env.Telegram.RequestsOf(telegramtest.AnswerCallbackQuery)
			assert.Empty(t, answers)
		})
	}
}
