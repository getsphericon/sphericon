package external_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The External API creates, edits, schedules and reports Telegram Broadcasts (ADR 0080).

func TestExternalTelegramBroadcastIsCreatedEditedAndReportedWithoutOpenOrDeliveryFigures(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	c := env.ExternalScoped(t, apitokens.ScopeBroadcastsRead, apitokens.ScopeBroadcastsWrite)

	made, err := c.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{
		Name:          "News",
		Content:       testhelper.ExternalTelegram("Hello <b>{{ first_name }}</b>"),
		IntegrationId: externalapi.NewOptNilEntityId(entityIDString(int64(bot.Integration.ID))),
	})
	require.NoError(t, err)
	read, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(made.ID))), Content: testhelper.ExternalTelegramEdit("Edited")},
		externalapi.BroadcastsUpdateParams{ID: made.ID})
	require.NoError(t, err)
	report, err := c.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: made.ID})
	require.NoError(t, err)

	assert.Equal(t, externalapi.BroadcastChannelTelegram, made.Channel)
	assert.Equal(t, "Hello <b>{{ first_name }}</b>", testhelper.ExternalTelegramOf(made).Text)
	assert.Equal(t, "Edited", testhelper.ExternalTelegramOf(read).Text)
	figures := report
	assert.True(t, figures.OpenedCount.Null)
	assert.True(t, figures.DeliveredCount.Null)
	assert.True(t, figures.OpenRate.Null)
	assert.Zero(t, figures.PendingCount)
}

func TestExternalTelegramBroadcastScheduleIsRefusedForARouteThatCannotSendAndForAnOverLongText(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	c := env.ExternalScoped(t, apitokens.ScopeBroadcastsRead, apitokens.ScopeBroadcastsWrite, apitokens.ScopeBroadcastsSend)
	create := func(text string, integrationID int64) externalapi.EntityId {
		in := &externalapi.CreateBroadcastInput{
			Name: "News", Content: testhelper.ExternalTelegram(text), IntegrationId: externalapi.NewOptNilEntityId(entityIDString(integrationID)),
		}
		b, err := c.BroadcastsCreate(ctx, in)
		require.NoError(t, err)
		return b.ID
	}
	schedule := func(id externalapi.EntityId) error {
		_, err := c.BroadcastsSchedule(ctx, &externalapi.ScheduleBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(id))), ScheduledAt: externalapi.Timestamp(time.Now().Add(time.Hour))},
			externalapi.BroadcastsScheduleParams{ID: id})
		return err
	}
	botID := int64(bot.Integration.ID)

	noDefault := schedule(entityIDString(fixtures.BroadcastTelegramDraftID))
	tooLong := schedule(create(strings.Repeat("a", 4097), botID))
	atTheLimit := schedule(create(strings.Repeat("a", 4096), botID))

	testhelper.ExternalRefusal(t, noDefault, http.StatusConflict, "the Channel has no default Integration to send through")
	testhelper.ExternalRefusal(t, tooLong, http.StatusUnprocessableEntity)
	assert.NoError(t, atTheLimit)
}

func TestExternalTelegramBroadcastHasNoTestSend(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.ExternalScoped(t, authorScopes...)

	err := c.BroadcastsTestSend(ctx, &externalapi.TestSendBroadcastInput{Email: externalapi.NewOptEmailAddress("qa@test.dev")},
		externalapi.BroadcastsTestSendParams{ID: entityIDString(fixtures.BroadcastTelegramDraftID)})
	refused := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Value, "no test send")
	assert.Empty(t, env.CustomerMail.Messages(), "nothing was sent")
}
