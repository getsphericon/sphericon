package jobs_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// release is the job a deletion of Acme's Telegram Integration queues.
var release = integrations.ReleaseWebhookArgs{WorkspaceID: fixtures.AcmeID, IntegrationID: fixtures.IntegrationAcmeTelegramID}

func TestReleaseWebhookLetsTheBotOfADeletedTelegramIntegrationGoOfItsWebhook(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeTelegramID).SetDiscardedAt(time.Now()).ExecX(t.Context())

	err := jobs.NewReleaseWebhookWorker(env.DB, env.Integrations).Work(t.Context(), job(release))

	require.NoError(t, err)
	deleted, ok := env.Telegram.Last(telegramtest.DeleteWebhook)
	require.True(t, ok, "the bot's webhook is deleted")
	assert.Equal(t, bot.Token, deleted.Token)
}

// The release is best effort: a refusal of Telegram is logged and the job ends, as the release on
// disabling a bot does.
func TestReleaseWebhookRefusedByTelegramEndsTheJob(t *testing.T) {
	env := testhelper.Setup(t)
	env.AcmeTelegram(t)
	env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeTelegramID).SetDiscardedAt(time.Now()).ExecX(t.Context())
	env.Telegram.FailOnce(telegramtest.DeleteWebhook, telegramtest.Unauthorized)

	err := jobs.NewReleaseWebhookWorker(env.DB, env.Integrations).Work(t.Context(), job(release))

	require.NoError(t, err)
	tried := env.Telegram.RequestsOf(telegramtest.DeleteWebhook)
	assert.Len(t, tried, 1)
}

// Only a deleted Integration's bot is released: called on a kept one, or on one gone with its
// Workspace, the job touches no bot.
func TestReleaseWebhookOfAnIntegrationNotDeletedTouchesNoBot(t *testing.T) {
	env := testhelper.Setup(t)
	env.AcmeTelegram(t)
	worker := jobs.NewReleaseWebhookWorker(env.DB, env.Integrations)

	kept := worker.Work(t.Context(), job(release))
	gone := worker.Work(t.Context(), job(integrations.ReleaseWebhookArgs{WorkspaceID: fixtures.AcmeID, IntegrationID: 999999}))

	require.NoError(t, kept)
	require.NoError(t, gone)
	assert.Empty(t, env.Telegram.RequestsOf(telegramtest.DeleteWebhook))
}
