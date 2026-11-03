package external_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The External API creates, edits, schedules and reports Telegram Broadcasts (ADR 0080).

func TestExternalTelegramBroadcastIsCreatedEditedAndReportedWithoutOpenOrDeliveryFigures(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.ConnectTelegram(t, 7501, "acme_api_bot")
	c := env.ExternalScoped(t, "broadcasts:read", "broadcasts:write")

	created, err := c.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{
		Name:          "News",
		Content:       testhelper.ExternalTelegram("Hello <b>{{ first_name }}</b>"),
		IntegrationId: externalapi.NewOptNilEntityId(entityIDString(int64(bot.Integration.ID))),
	})
	require.NoError(t, err)
	made, ok := created.(*externalapi.BroadcastResource)
	require.Truef(t, ok, "got %T", created)
	edited, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Content: testhelper.ExternalTelegramEdit("Edited")},
		externalapi.BroadcastsUpdateParams{ID: made.ID})
	require.NoError(t, err)
	read, ok := edited.(*externalapi.BroadcastResource)
	require.Truef(t, ok, "got %T", edited)
	report, err := c.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: made.ID})
	require.NoError(t, err)

	assert.Equal(t, externalapi.BroadcastChannelTelegram, made.Channel)
	assert.Equal(t, "Hello <b>{{ first_name }}</b>", testhelper.ExternalTelegramOf(made).Text)
	assert.Equal(t, "Edited", testhelper.ExternalTelegramOf(read).Text)
	figures, ok := report.(*externalapi.BroadcastReport)
	require.Truef(t, ok, "got %T", report)
	assert.True(t, figures.OpenedCount.Null)
	assert.True(t, figures.DeliveredCount.Null)
	assert.True(t, figures.OpenRate.Null)
	assert.Zero(t, figures.PendingCount)
}

func TestExternalTelegramBroadcastScheduleIsRefusedForARouteThatCannotSendAndForAnOverLongText(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.ConnectTelegram(t, 7502, "acme_sched_bot")
	c := env.ExternalScoped(t, "broadcasts:read", "broadcasts:write", "broadcasts:send")
	create := func(text string, integrationID *int64) externalapi.EntityId {
		in := &externalapi.CreateBroadcastInput{Name: "News", Content: testhelper.ExternalTelegram(text)}
		if integrationID != nil {
			in.IntegrationId = externalapi.NewOptNilEntityId(entityIDString(*integrationID))
		}
		res, err := c.BroadcastsCreate(ctx, in)
		require.NoError(t, err)
		b, ok := res.(*externalapi.BroadcastResource)
		require.Truef(t, ok, "got %T", res)
		return b.ID
	}
	schedule := func(id externalapi.EntityId) externalapi.BroadcastsScheduleRes {
		res, err := c.BroadcastsSchedule(ctx, &externalapi.ScheduleBroadcastInput{ScheduledAt: externalapi.Timestamp(time.Now().Add(time.Hour))},
			externalapi.BroadcastsScheduleParams{ID: id})
		require.NoError(t, err)
		return res
	}
	botID := int64(bot.Integration.ID)

	noDefault := schedule(create("Hello", nil))
	tooLong := schedule(create(strings.Repeat("a", 4097), &botID))
	atTheLimit := schedule(create(strings.Repeat("a", 4096), &botID))

	assert.IsType(t, &externalapi.BroadcastsScheduleConflict{}, noDefault, "the Channel has no default Integration to send through")
	assert.IsType(t, &externalapi.BroadcastsScheduleUnprocessableEntity{}, tooLong)
	assert.IsType(t, &externalapi.BroadcastResource{}, atTheLimit)
}

func TestExternalTelegramBroadcastHasNoTestSend(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.ConnectTelegram(t, 7503, "acme_test_bot")
	c := env.ExternalScoped(t, authorScopes...)
	res, err := c.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{
		Name: "News", Content: testhelper.ExternalTelegram("Hello"),
		IntegrationId: externalapi.NewOptNilEntityId(entityIDString(int64(bot.Integration.ID))),
	})
	require.NoError(t, err)
	b, ok := res.(*externalapi.BroadcastResource)
	require.Truef(t, ok, "got %T", res)

	out, err := c.BroadcastsTestSend(ctx, &externalapi.TestSendBroadcastInput{Email: "qa@test.dev"}, externalapi.BroadcastsTestSendParams{ID: b.ID})
	require.NoError(t, err)

	refused, ok := out.(*externalapi.BroadcastsTestSendUnprocessableEntity)
	require.Truef(t, ok, "got %T", out)
	assert.Contains(t, refused.Detail.Value, "no test send")
	assert.Empty(t, env.CustomerMail.Messages(), "nothing was sent")
}
