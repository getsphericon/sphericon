package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The bot the Telegram tests connect, as the fake Bot API knows it.
const (
	acmeBotID       = 7000000001
	acmeBotUsername = "acme_news_bot"
)

func telegramInput(token string) siteapi.IntegrationConfigInput {
	return siteapi.IntegrationConfigInput{
		OneOf: siteapi.NewTelegramConfigInputIntegrationConfigInputSum(siteapi.TelegramConfigInput{
			Kind:  siteapi.TelegramConfigInputKindTelegram,
			Token: token,
		}),
	}
}

func createTelegram(t *testing.T, c *siteapi.Client, token string) (*siteapi.SiteIntegrationResource, error) {
	t.Helper()
	return c.SiteIntegrationsCreate(context.Background(), &siteapi.CreateIntegrationInput{
		Name: "Acme bot", Config: telegramInput(token),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
}

func updateTelegram(t *testing.T, c *siteapi.Client, id siteapi.EntityId, in *siteapi.UpdateIntegrationInput) (*siteapi.SiteIntegrationResource, error) {
	t.Helper()
	return c.SiteIntegrationsUpdate(context.Background(), in, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
}

// connectedTelegram connects the Acme bot through the Site API and returns the Integration and the
// bot's token.
func connectedTelegram(t *testing.T, env *testhelper.TestEnv, c *siteapi.Client) (row *siteapi.SiteIntegrationResource, token string) {
	t.Helper()
	token = env.Telegram.AddBot(acmeBotID, acmeBotUsername)
	row, err := createTelegram(t, c, token)
	require.NoError(t, err)
	return row, token
}

// problemOf is the field errors of the 422 err refused a call with.
func problemOf(t *testing.T, err error) siteapi.ProblemDetailsErrors {
	t.Helper()
	return testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity).Errors.Value
}

func TestSiteTelegramConnectsABot(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	row, token := connectedTelegram(t, env, c)

	cfg, ok := row.Config.OneOf.GetSiteTelegramConfig()
	require.Truef(t, ok, "config is telegram, got %v", row.Config.OneOf.Type)
	assert.Equal(t, acmeBotUsername, cfg.Username)
	assert.Equal(t, int64(acmeBotID), cfg.BotId)
	assert.Equal(t, siteapi.SiteIntegrationChannelTelegram, row.Channel)
	assert.Equal(t, siteapi.SiteIntegrationProviderTelegram, row.Provider)
	assert.Equal(t, "telegram", row.Preset.Or(""))
	assert.Equal(t, siteapi.NewNilInt32(30), row.ProviderLimit.PerSecond.Limit)
	assert.Equal(t, siteapi.NewNilSiteProviderLimitSource(siteapi.SiteProviderLimitSourceProvider), row.ProviderLimit.PerSecond.Source)
	assert.True(t, row.ProviderLimit.PerDay.Limit.Null, "no 24-hour value")

	calls := env.Telegram.Requests()
	require.Len(t, calls, 2)
	assert.Equal(t, telegramtest.GetMe, calls[0].Method)
	hook := calls[1]
	assert.Equal(t, telegramtest.SetWebhook, hook.Method)
	assert.Equal(t, token, hook.Token)
	assert.Equal(t, row.HookUrl, hook.String("url"))
	assert.NotEmpty(t, hook.String("secret_token"))
	assert.Equal(t, []string{"message", "callback_query", "my_chat_member"}, hook.Strings("allowed_updates"))
	assert.True(t, hook.Bool("drop_pending_updates"), "the first connect clears another tool's backlog")
}

func TestSiteTelegramRefusesWhatTelegramRefuses(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	t.Run("a token Telegram does not accept", func(t *testing.T) {
		_, err := createTelegram(t, c, env.Telegram.UnknownToken(acmeBotID))

		errs := problemOf(t, err)
		assert.Contains(t, errs, "config.token")
	})

	t.Run("a token that is not shaped like one", func(t *testing.T) {
		_, err := createTelegram(t, c, "not-a-token")

		errs := problemOf(t, err)
		assert.Contains(t, errs, "config.token")
	})

	t.Run("a webhook Telegram refuses", func(t *testing.T) {
		token := env.Telegram.AddBot(acmeBotID+1, "other_bot")
		env.Telegram.FailOnce(telegramtest.SetWebhook, telegramtest.BadWebhook)

		_, err := createTelegram(t, c, token)

		errs := problemOf(t, err)
		require.Contains(t, errs, "config")
		assert.Contains(t, errs["config"][0], "An HTTPS URL must be provided for webhook")
	})

	list, err := c.SiteIntegrationsList(context.Background(), siteapi.SiteIntegrationsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	bots := lo.FilterMap(list, func(row siteapi.SiteIntegrationResource, _ int) (int64, bool) {
		return int64(row.ID), row.Provider == siteapi.SiteIntegrationProviderTelegram
	})
	assert.ElementsMatch(t, []int64{fixtures.IntegrationAcmeTelegramID, fixtures.IntegrationAcmeSecondTelegramID}, bots,
		"a refused connect saved nothing: only the fixture bots are there")
}

func TestSiteTelegramUpdateReconnectsTheSameBotOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	row, _ := connectedTelegram(t, env, c)

	t.Run("a token of another bot is refused", func(t *testing.T) {
		other := env.Telegram.AddBot(acmeBotID+1, "other_bot")

		_, err := updateTelegram(t, c, row.ID, &siteapi.UpdateIntegrationInput{Config: siteapi.NewOptIntegrationConfigInput(telegramInput(other))})

		errs := problemOf(t, err)
		assert.Equal(t, []string{"this token belongs to another bot; add a new Integration"}, errs["config.token"])
	})

	t.Run("a new token of the same bot re-registers the webhook", func(t *testing.T) {
		rotated := env.Telegram.Rotate(acmeBotID)

		updated, err := updateTelegram(t, c, row.ID, &siteapi.UpdateIntegrationInput{Config: siteapi.NewOptIntegrationConfigInput(telegramInput(rotated))})

		require.NoError(t, err)
		hook, ok := env.Telegram.Last(telegramtest.SetWebhook)
		require.True(t, ok)
		assert.Equal(t, rotated, hook.Token)
		assert.Equal(t, updated.HookUrl, hook.String("url"))
		assert.False(t, hook.Bool("drop_pending_updates"), "a later registration keeps the pending updates")
	})

	t.Run("a blank token keeps the stored one and calls nothing", func(t *testing.T) {
		before := len(env.Telegram.Requests())

		_, err := updateTelegram(t, c, row.ID, &siteapi.UpdateIntegrationInput{
			Name: siteapi.NewOptName("Renamed bot"), Config: siteapi.NewOptIntegrationConfigInput(telegramInput("")),
		})

		require.NoError(t, err)
		assert.Len(t, env.Telegram.Requests(), before)
	})
}

// A deleted Telegram Integration is read-only (ADR 0100): enabling it, giving it a new token of its
// bot or a new hook key answers 404, and nothing reaches Telegram after the delete.
func TestSiteTelegramDeletedIntegrationTouchesNoBot(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	row, _ := connectedTelegram(t, env, c)
	err := c.SiteIntegrationsDelete(ctx, siteapi.SiteIntegrationsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: row.ID})
	require.NoError(t, err)
	before := len(env.Telegram.Requests())

	_, err = updateTelegram(t, c, row.ID, &siteapi.UpdateIntegrationInput{
		Enabled: siteapi.NewOptBool(true), Config: siteapi.NewOptIntegrationConfigInput(telegramInput(env.Telegram.Rotate(acmeBotID))),
	})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	_, err = c.SiteIntegrationsRegenerateKey(ctx, siteapi.SiteIntegrationsRegenerateKeyParams{WorkspaceId: fixtures.AcmePublicID, ID: row.ID})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	assert.Len(t, env.Telegram.Requests(), before, "no Bot API call after the delete")
}

func TestSiteTelegramWebhookFollowsTheIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	row, token := connectedTelegram(t, env, c)
	path := siteapi.SiteIntegrationsRegenerateKeyParams{WorkspaceId: fixtures.AcmePublicID, ID: row.ID}

	t.Run("regenerating the key re-registers the webhook", func(t *testing.T) {
		regenerated, err := c.SiteIntegrationsRegenerateKey(ctx, path)
		require.NoError(t, err)

		assert.NotEqual(t, row.HookUrl, regenerated.HookUrl)
		hook, ok := env.Telegram.Last(telegramtest.SetWebhook)
		require.True(t, ok)
		assert.Equal(t, regenerated.HookUrl, hook.String("url"))
		assert.False(t, hook.Bool("drop_pending_updates"))
	})

	t.Run("a webhook Telegram refuses keeps the old key", func(t *testing.T) {
		env.Telegram.FailOnce(telegramtest.SetWebhook, telegramtest.BadWebhook)

		_, err := c.SiteIntegrationsRegenerateKey(ctx, path)

		problemOf(t, err)
	})

	t.Run("disabling deletes the webhook", func(t *testing.T) {
		_, err := updateTelegram(t, c, row.ID, &siteapi.UpdateIntegrationInput{Enabled: siteapi.NewOptBool(false)})

		require.NoError(t, err)
		deleted, ok := env.Telegram.Last(telegramtest.DeleteWebhook)
		require.True(t, ok)
		assert.Equal(t, token, deleted.Token)
	})

	t.Run("enabling again re-registers it without dropping updates", func(t *testing.T) {
		before := len(env.Telegram.RequestsOf(telegramtest.SetWebhook))

		_, err := updateTelegram(t, c, row.ID, &siteapi.UpdateIntegrationInput{Enabled: siteapi.NewOptBool(true)})

		require.NoError(t, err)
		hooks := env.Telegram.RequestsOf(telegramtest.SetWebhook)
		require.Len(t, hooks, before+1)
		assert.False(t, hooks[before].Bool("drop_pending_updates"))
	})

	t.Run("archiving deletes the webhook, and a failure does not block it", func(t *testing.T) {
		env.Telegram.FailOnce(telegramtest.DeleteWebhook, telegramtest.Unauthorized)
		before := len(env.Telegram.RequestsOf(telegramtest.DeleteWebhook))

		err := c.SiteIntegrationsDelete(ctx, siteapi.SiteIntegrationsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: row.ID})
		require.NoError(t, err)

		assert.Len(t, env.Telegram.RequestsOf(telegramtest.DeleteWebhook), before+1)
	})
}

func TestSiteTelegramHookChecksTheSecret(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	update := `{"update_id":10000,"message":{"message_id":1,"date":1767225600,"chat":{"id":42,"type":"private"},"from":{"id":42,"is_bot":false,"first_name":"Alice"},"text":"hello"}}`

	impostor := bot
	impostor.Secret = "not-the-secret"
	refused := env.PostTelegramUpdate(t, impostor, update)
	accepted := env.PostTelegramUpdate(t, bot, update)

	assert.Equal(t, http.StatusForbidden, refused.Code)
	assert.Equal(t, http.StatusOK, accepted.Code)
}

func TestSiteTelegramPresetIsFixed(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := siteapi.EntityId(fixtures.IntegrationAcmeTelegramID)

	t.Run("a Telegram integration takes no other Preset", func(t *testing.T) {
		_, err := updateTelegram(t, c, id, &siteapi.UpdateIntegrationInput{Preset: siteapi.NewOptNilString("generic")})

		assert.Contains(t, problemOf(t, err), "preset")
	})

	t.Run("no other integration takes the telegram Preset", func(t *testing.T) {
		_, err := c.SiteIntegrationsCreate(ctx, &siteapi.CreateIntegrationInput{
			Name: "SMTP", Config: smtpInput("smtp.acme.test", "pw"), Preset: siteapi.NewOptNilString("telegram"),
		}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})

		assert.Contains(t, problemOf(t, err), "preset")
	})

	t.Run("the Presets offered leave it out", func(t *testing.T) {
		presets, err := c.SitePresetsList(ctx)
		require.NoError(t, err)

		names := make([]string, 0, len(presets))
		for _, p := range presets {
			names = append(names, p.Name)
		}
		assert.NotContains(t, names, "telegram")
	})
}
