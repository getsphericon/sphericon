package server_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/server"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/testhelper/metricstest"
)

// SES bounce and complaint notifications are counted by provider and status.
func TestSESHookCountsBounceAndComplaintOutcomes(t *testing.T) {
	env := testhelper.Setup(t)
	metricstest.StartMetrics(t)
	h := server.NewHooks(env.DB, env.Bus, env.Integrations, acceptAll, noConfirm)

	bounce := `{"notificationType":"Bounce","mail":{"source":"hello@codebasics.dev"},
	  "bounce":{"bounceType":"Permanent","bouncedRecipients":[{"emailAddress":"` + strings.ToUpper(fixtures.ContactAliceEmail) + `"}]}}`
	require.Equal(t, 200, postSES(t, h, fixtures.IntegrationAcmeSesIngestKey, sesNotificationBody(t, bounce)).Code)
	complaint := `{"eventType":"Complaint","complaint":{"complainedRecipients":[{"emailAddress":"stranger@example.com"}]}}`
	require.Equal(t, 200, postSES(t, h, fixtures.IntegrationAcmeSesIngestKey, sesNotificationBody(t, complaint)).Code)

	persistOutbox(t, env)

	scrape := metricstest.ScrapeMetrics(t)
	assert.InDelta(t, 1, scrape.Value(t, "email_send_outcomes_total", map[string]string{"provider": "ses", "status": "bounce"}), 0)
	assert.InDelta(t, 1, scrape.Value(t, "email_send_outcomes_total", map[string]string{"provider": "ses", "status": "complaint"}), 0)
}

// An SNS redelivery of the same notification (same MessageId) is published twice
// but persisted once, and counted once.
func TestSESHookRedeliveryCountsOutcomeOnce(t *testing.T) {
	env := testhelper.Setup(t)
	metricstest.StartMetrics(t)
	h := server.NewHooks(env.DB, env.Bus, env.Integrations, acceptAll, noConfirm)

	bounce := `{"notificationType":"Bounce","mail":{"source":"hello@codebasics.dev"},
	  "bounce":{"bounceType":"Permanent","bouncedRecipients":[{"emailAddress":"` + fixtures.ContactAliceEmail + `"}]}}`
	body := sesNotificationBody(t, bounce)
	require.Equal(t, 200, postSES(t, h, fixtures.IntegrationAcmeSesIngestKey, body).Code)
	require.Equal(t, 200, postSES(t, h, fixtures.IntegrationAcmeSesIngestKey, body).Code)
	persistOutbox(t, env)

	scrape := metricstest.ScrapeMetrics(t)
	assert.InDelta(t, 1, scrape.Value(t, "email_send_outcomes_total", map[string]string{"provider": "ses", "status": "bounce"}), 0)
}

// persistOutbox plays the persist consumer over everything published so far.
func persistOutbox(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	for _, envelope := range env.OutboxEnvelopes(t, events.NameEmailBounced, events.NameEmailComplained) {
		require.NoError(t, events.Persist(t.Context(), env.DB, envelope))
	}
}
