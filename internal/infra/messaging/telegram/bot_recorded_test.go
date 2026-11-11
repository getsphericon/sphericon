package telegram_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram/telegramtest"
)

// The provider against the real Bot API (recorded_test.go). The refusals are asserted against the
// fake Bot API's (telegramtest), which the rest of the suite sends through: what the classification
// of a failed send reads must be what Telegram says.

func TestRecordedGetMeNamesTheBot(t *testing.T) {
	r := recorded(t)

	info, err := r.bot.GetMe(t.Context())

	require.NoError(t, err)
	assert.Equal(t, messaging.TelegramBotInfo{ID: r.botID, Username: r.botUsername}, info)
}

func TestRecordedWebhookIsSetThenDeleted(t *testing.T) {
	r := recorded(t)

	require.NoError(t, r.bot.SetWebhook(t.Context(), messaging.TelegramWebhook{
		URL:                "https://example.com/hooks/telegram",
		SecretToken:        "recorded-secret_1",
		AllowedUpdates:     []string{"message", "callback_query", "my_chat_member"},
		DropPendingUpdates: true,
	}))
	require.NoError(t, r.bot.DeleteWebhook(t.Context()))
}

func TestRecordedSendMessageWithButtonsIsAccepted(t *testing.T) {
	r := recorded(t)

	sent, err := r.bot.SendMessage(t.Context(), messaging.TelegramMessage{
		ChatID:            r.chatID,
		Text:              `<b>Recorded</b> message with a <a href="https://example.com/offer">link</a>`,
		Buttons:           []messaging.TelegramButton{{Label: "Open the offer", URL: "https://example.com/offer"}},
		NoLinkPreview:     true,
		UnsubscribeButton: "Unsubscribe",
	})

	require.NoError(t, err)
	assert.Positive(t, sent.MessageID)
}

func TestRecordedSendMessageWithAnImageIsAPhoto(t *testing.T) {
	r := recorded(t)

	sent, err := r.bot.SendMessage(t.Context(), messaging.TelegramMessage{
		ChatID:   r.chatID,
		Text:     "<i>Recorded</i> photo caption",
		ImageURL: "https://telegram.org/img/t_logo.png",
	})

	require.NoError(t, err)
	assert.Positive(t, sent.MessageID)
}

func TestRecordedWrongTokenIsUnauthorized(t *testing.T) {
	r := recorded(t)
	wrong, err := telegram.New(telegram.DefaultAPIURL, "987654321:BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB", telegram.WithTransport(r.transport))
	require.NoError(t, err)

	_, err = wrong.GetMe(t.Context())

	requireRefusal(t, telegramtest.Unauthorized, err)
	require.ErrorIs(t, err, messaging.ErrTelegramUnauthorized)
}

func TestRecordedUnknownChatIsNotFound(t *testing.T) {
	r := recorded(t)

	_, err := r.bot.SendMessage(t.Context(), messaging.TelegramMessage{ChatID: "1", Text: "nobody"})

	requireRefusal(t, telegramtest.ChatNotFound, err)
}

func TestRecordedUnknownTagIsBadEntities(t *testing.T) {
	r := recorded(t)

	_, err := r.bot.SendMessage(t.Context(), messaging.TelegramMessage{ChatID: r.chatID, Text: "<foo>"})

	requireRefusal(t, telegramtest.BadEntities, err)
}

func TestRecordedPlainHTTPWebhookIsBadWebhook(t *testing.T) {
	r := recorded(t)

	err := r.bot.SetWebhook(t.Context(), messaging.TelegramWebhook{URL: "http://example.com/hooks/telegram"})

	requireRefusal(t, telegramtest.BadWebhook, err)
}

// requireRefusal checks that err is the Bot API's refusal want, word for word.
func requireRefusal(t *testing.T, want telegramtest.Failure, err error) {
	t.Helper()
	te, ok := errors.AsType[*messaging.TelegramError](err)
	require.True(t, ok, "a refusal of the Bot API, got %v", err)
	assert.Equal(t, want.Code, te.Code)
	assert.Equal(t, want.Description, te.Description)
}
