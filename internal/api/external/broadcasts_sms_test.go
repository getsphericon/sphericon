package external_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/infra/messaging/twilio/twiliotest"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The External API (and MCP, projected from it) creates, schedules, reads and Test sends an SMS
// Broadcast (ADR 0016, 0080); an Integration a scheduled SMS Broadcast sends through cannot be
// deleted.
func TestExternalSMSBroadcastIsCreatedTestSentScheduledAndKeepsItsIntegrationInUse(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.AcmeTwilio(t)
	twilio := int64(account.Integration.ID)
	c := env.ExternalScoped(t, "broadcasts:read", "broadcasts:write", "broadcasts:send", "integrations:read", "integrations:write")

	made, err := c.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{
		Name: "Flash sale", Content: testhelper.ExternalSMS("Hi {{ first_name }}"),
		IntegrationId: externalapi.NewOptNilEntityId(entityIDString(twilio)),
	})
	require.NoError(t, err)
	tested := c.BroadcastsTestSend(ctx, &externalapi.TestSendBroadcastInput{Phone: externalapi.NewOptString("+14155550199")},
		externalapi.BroadcastsTestSendParams{ID: made.ID})
	badNumber := c.BroadcastsTestSend(ctx, &externalapi.TestSendBroadcastInput{Phone: externalapi.NewOptString("not a number")},
		externalapi.BroadcastsTestSendParams{ID: made.ID})
	_, err = c.BroadcastsSchedule(ctx, &externalapi.ScheduleBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(made.ID))), ScheduledAt: externalapi.Timestamp(time.Now().Add(time.Hour))},
		externalapi.BroadcastsScheduleParams{ID: made.ID})
	require.NoError(t, err)
	report, err := c.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: made.ID})
	require.NoError(t, err)
	deleted := c.IntegrationsDelete(ctx, externalapi.IntegrationsDeleteParams{ID: entityIDString(twilio)})

	assert.Equal(t, externalapi.BroadcastChannelSMS, made.Channel)
	assert.Equal(t, "Hi {{ first_name }}", testhelper.ExternalSMSOf(made).Text)
	require.NoError(t, tested)
	assert.Equal(t, []string{"+14155550199"}, lo.Map(env.Twilio.Messages(), func(m twiliotest.Message, _ int) string { return m.To }))
	testhelper.ExternalRefusal(t, badNumber, http.StatusUnprocessableEntity)
	figures := report
	assert.True(t, figures.OpenedCount.Null)
	assert.Equal(t, externalapi.NewNilInt32(0), figures.DeliveredCount, "SMS reports deliveries, none yet")
	assert.Equal(t, externalapi.NewNilInt32(0), figures.UndeliveredCount)
	assert.True(t, figures.BouncedCount.Null)
	assert.True(t, figures.ComplainedCount.Null)
	users := usersOfRefusal(t, deleted)
	assert.Contains(t, lo.Map(users, func(u externalapi.UsedBy, _ int) externalapi.EntityId { return u.ID }), made.ID,
		"the scheduled SMS Broadcast sends through it")
}
