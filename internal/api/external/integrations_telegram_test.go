package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestExternalTelegramConnectsABot(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "integrations:write")
	token := env.Telegram.AddBot(7000000002, "acme_api_bot")

	res, err := c.IntegrationsCreate(context.Background(), &externalapi.CreateIntegrationInput{
		Name: "Bot", Config: externalapi.IntegrationConfigInput{OneOf: externalapi.NewTelegramConfigInputIntegrationConfigInputSum(
			externalapi.TelegramConfigInput{Kind: externalapi.TelegramConfigInputKindTelegram, Token: token})},
	})
	require.NoError(t, err)

	created := res
	cfg, ok := created.Config.OneOf.GetTelegramConfig()
	require.True(t, ok)
	assert.Equal(t, "acme_api_bot", cfg.Username)
	assert.Equal(t, externalapi.IntegrationChannelTelegram, created.Channel)
	hook, ok := env.Telegram.Last(telegramtest.SetWebhook)
	require.True(t, ok)
	assert.Equal(t, created.HookUrl, hook.String("url"))
}

func TestExternalPresetsLeaveOutTheFixedTelegramPreset(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.ExternalScoped(t, "integrations:read").PresetsList(context.Background())
	require.NoError(t, err)

	page := res
	names := make([]string, 0, len(page.Items))
	for _, p := range page.Items {
		names = append(names, p.Name)
	}
	assert.NotContains(t, names, "telegram")
	assert.Contains(t, names, "ses")
}
