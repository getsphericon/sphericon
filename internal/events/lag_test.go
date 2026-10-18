package events_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/testhelper/metricstest"
)

// Outbox lag is the age of the oldest row past each group's cursor, one sample
// per registered consumer group, labelled by group only.
func TestOutboxLagIsReportedPerConsumerGroup(t *testing.T) {
	env := testhelper.Setup(t)
	metricstest.StartMetrics(t)
	unregister, err := events.RegisterLagGauge(env.DB)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unregister() })

	err = env.Bus.WithinTx(t.Context(), func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(t.Context(), &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: 7, Email: "lag@example.com"})
	})
	require.NoError(t, err)
	env.AgeOutbox(t, 2*time.Minute)
	rows := env.OutboxRows(t)
	last := rows[len(rows)-1]
	env.SetOutboxCursor(t, events.GroupPersist, last.TxID, &last.Offset)

	scrape := metricstest.ScrapeMetrics(t)
	assert.Zero(t, scrape.Value(t, "outbox_lag_seconds", map[string]string{"consumer_group": events.GroupPersist}), "caught up")
	for _, g := range []string{events.GroupAutomations, events.GroupWebhooks, events.GroupSuppression} {
		assert.GreaterOrEqual(t, scrape.Value(t, "outbox_lag_seconds", map[string]string{"consumer_group": g}), 120.0, g)
	}
}
