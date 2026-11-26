package telegram_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram"
)

const token = "123456789:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func unreachableBot(t *testing.T) *telegram.Bot {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing listens at url any more: the connection fails
	b, err := telegram.New(url, token)
	require.NoError(t, err)
	return b
}

func TestConnectionFailureNeverExposesTheToken(t *testing.T) {
	b := unreachableBot(t)

	_, err := b.GetMe(t.Context())
	require.Error(t, err)
	require.NotContains(t, err.Error(), token)
	require.NotContains(t, err.Error(), "123456789:")

	_, err = b.SendMessage(t.Context(), messaging.TelegramMessage{ChatID: "42", Text: "hi"})
	require.Error(t, err)
	require.NotContains(t, err.Error(), token, err.Error())

	require.NotContains(t, b.DeleteWebhook(t.Context()).Error(), token)
}
