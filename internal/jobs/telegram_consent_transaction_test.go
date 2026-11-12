package jobs_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestTelegramResumeFailureRollsBackTokenBindingConsentAndReceipt(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	stop := env.PostTelegramUpdate(t, bot, said(1, optOutUser, "/stop"))
	require.Equal(t, http.StatusOK, stop.Code)
	fail := true
	env.Bus.UseSubscribers(events.Subscriber{
		Wants: func(_ events.Envelope, ev events.DomainEvent) bool {
			_, ok := ev.(*events.TelegramResumeObserved)
			return ok
		},
		Handle: func(context.Context, *ent.Scoped, events.Publisher, events.Envelope, events.DomainEvent) error {
			if fail {
				return errors.New("resume reaction failed")
			}
			return nil
		},
	})
	update := said(2, optOutUser, "/start "+fixtures.StartTokenAcmeOpenToken)

	failed := env.PostTelegramUpdate(t, bot, update)

	require.Equal(t, http.StatusInternalServerError, failed.Code)
	token := env.DB.StartToken.GetX(t.Context(), fixtures.StartTokenAcmeOpenID)
	assert.Nil(t, token.UsedAt)
	bound := env.DB.Binding.Query().Where(binding.Destination("4101")).ExistX(t.Context())
	assert.False(t, bound)
	stopped := optedOut(t, env, optOutUser)
	assert.True(t, stopped)
	boundEvents := env.QueuedEnvelopes(t, systemevent.TelegramBound)
	assert.Empty(t, boundEvents)
	resumes := env.QueuedEnvelopes(t, events.NameTelegramResumeObserved)
	assert.Empty(t, resumes)
	fail = false

	retry := env.PostTelegramUpdate(t, bot, update)

	require.Equal(t, http.StatusOK, retry.Code)
	token = env.DB.StartToken.GetX(t.Context(), fixtures.StartTokenAcmeOpenID)
	assert.NotNil(t, token.UsedAt)
	bound = env.DB.Binding.Query().Where(binding.Destination("4101"), binding.ContactID(fixtures.ContactLiamID)).ExistX(t.Context())
	assert.True(t, bound)
	stopped = optedOut(t, env, optOutUser)
	assert.False(t, stopped)
	boundCount := env.QueuedCount(t, systemevent.TelegramBound, map[string]any{"contactId": fixtures.ContactLiamID})
	assert.Equal(t, 1, boundCount)
	stop = env.PostTelegramUpdate(t, bot, said(3, optOutUser, "/stop"))
	require.Equal(t, http.StatusOK, stop.Code)
	duplicate := env.PostTelegramUpdate(t, bot, update)
	require.Equal(t, http.StatusOK, duplicate.Code)
	stopped = optedOut(t, env, optOutUser)
	assert.True(t, stopped, "a used token and duplicate resume cannot undo a later stop")
}

// Acknowledgements have no database state by design (ADR 0114). Observe the external Bot API
// HTTP boundary, including redelivery and rollback, rather than an internal collaborator.
func TestTelegramButtonRollbackRetryAndDuplicateAcknowledgement(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	user := mustInt(t, fixtures.BindingLiamDestination)
	// The fixture's parked message has been received through this bot in this scenario.
	env.DB.OutboundMessage.UpdateOneID(fixtures.OutboundMessageParkedClaimID).
		SetStatus(outboundmessage.StatusSent).SetChannel(channels.Telegram).
		SetContactID(fixtures.ContactLiamID).SetDestination(fixtures.BindingLiamDestination).
		SetIntegrationID(int64(bot.Integration.ID)).SetProviderMessageID("77").ExecX(t.Context())
	fail := true
	env.Bus.UseSubscribers(events.Subscriber{
		Wants: func(_ events.Envelope, ev events.DomainEvent) bool {
			_, ok := ev.(*events.TelegramStopObserved)
			return ok
		},
		Handle: func(context.Context, *ent.Scoped, events.Publisher, events.Envelope, events.DomainEvent) error {
			if fail {
				return errors.New("stop reaction failed")
			}
			return nil
		},
	})
	update := pressed(5, user, 77, "cb-retry", "unsubscribe")

	failed := env.PostTelegramUpdate(t, bot, update)

	require.Equal(t, http.StatusInternalServerError, failed.Code)
	stopped := optedOut(t, env, user)
	assert.False(t, stopped)
	transitions := env.QueuedEnvelopes(t, systemevent.TelegramUnsubscribed)
	assert.Empty(t, transitions)
	attribution := env.JobsOf(t, jobs.BroadcastUnsubscribeEventArgs{}.Kind())
	assert.Empty(t, attribution)
	answers := env.Telegram.RequestsOf(telegramtest.AnswerCallbackQuery)
	assert.Empty(t, answers, "rolled-back facts are never acknowledged")
	fail = false

	retry := env.PostTelegramUpdate(t, bot, update)

	require.Equal(t, http.StatusOK, retry.Code)
	stopped = optedOut(t, env, user)
	assert.True(t, stopped)
	attributed := env.QueuedCount(t, systemevent.TelegramUnsubscribed, map[string]any{
		"contactId": fixtures.ContactLiamID, "broadcastId": fixtures.BroadcastArchivedID, "via": events.UnsubscribedByButton,
	})
	assert.Equal(t, 1, attributed)
	start := env.PostTelegramUpdate(t, bot, said(6, user, "/start"))
	require.Equal(t, http.StatusOK, start.Code)
	duplicate := env.PostTelegramUpdate(t, bot, update)
	require.Equal(t, http.StatusOK, duplicate.Code)
	stopped = optedOut(t, env, user)
	assert.False(t, stopped)
	answers = env.Telegram.RequestsOf(telegramtest.AnswerCallbackQuery)
	assert.Len(t, answers, 2, "successful retries and duplicate buttons are both answered")
	env.Telegram.Fail(telegramtest.AnswerCallbackQuery, telegramtest.BadRequest("query is too old"))
	expired := env.PostTelegramUpdate(t, bot, update)
	require.Equal(t, http.StatusOK, expired.Code)
	stopped = optedOut(t, env, user)
	assert.False(t, stopped)
	transitions = env.QueuedEnvelopes(t, systemevent.TelegramUnsubscribed)
	assert.Len(t, transitions, 1)
	attribution = env.JobsOf(t, jobs.BroadcastUnsubscribeEventArgs{}.Kind())
	assert.Len(t, attribution, 1)
	observations := env.QueuedEnvelopes(t, events.NameTelegramStopObserved)
	assert.Len(t, observations, 1)
}
