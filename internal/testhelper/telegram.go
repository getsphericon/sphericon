package testhelper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
)

// TelegramBot is a bot connected as a Telegram Integration of Acme through the Site API, as the
// fake Bot API (env.Telegram) knows it.
type TelegramBot struct {
	// Integration is the Integration as the create answered it.
	Integration *siteapi.SiteIntegrationResource
	ID          int64
	Username    string
	Token       string
	// HookPath is the path of the Integration's Ingest hook, and Secret the webhook secret
	// Telegram sends back with every update: what PostTelegramUpdate posts with.
	HookPath string
	Secret   string
}

// ConnectTelegram adds a bot of id and username to the fake Bot API and connects it as a Telegram
// Integration of Acme, as its owner does in the app. The webhook secret is read from the fake's
// record of setWebhook: the API never shows it.
func (env *TestEnv) ConnectTelegram(t *testing.T, id int64, username string) TelegramBot {
	t.Helper()
	token := env.Telegram.AddBot(id, username)
	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteIntegrationsCreate(context.Background(), &siteapi.CreateIntegrationInput{
		Name: username,
		Config: siteapi.IntegrationConfigInput{OneOf: siteapi.NewTelegramConfigInputIntegrationConfigInputSum(
			siteapi.TelegramConfigInput{Kind: siteapi.TelegramConfigInputKindTelegram, Token: token})},
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	row, ok := res.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "connect the Telegram bot: got %T", res)
	hookURL, secret, ok := env.Telegram.Webhook(id)
	require.True(t, ok, "the bot's webhook was set")
	return TelegramBot{
		Integration: row, ID: id, Username: username, Token: token,
		HookPath: strings.TrimPrefix(hookURL, env.AppURL), Secret: secret,
	}
}

// PostTelegramUpdate posts a Bot API update (its JSON) to the bot's Ingest hook as Telegram does,
// with the webhook's secret header, and returns the response.
func (env *TestEnv) PostTelegramUpdate(t *testing.T, bot TelegramBot, update string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, bot.HookPath, strings.NewReader(update))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", bot.Secret)
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}
