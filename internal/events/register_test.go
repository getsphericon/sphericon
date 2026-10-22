package events_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/messaging/registry"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// RegisterSubscribers wires the four consumers onto one router; a published hard
// bounce reaches every one of them. It runs on a database of its own (the router
// reads the committed outbox), dropped with the test.
func TestRegisterSubscribersFansEveryEventOutToAllConsumers(t *testing.T) {
	sqlDB := testhelper.RealDB(t)
	client := db.NewEntClient(sqlDB)

	ctx := context.Background()
	const dest = "registered-bounce@example.com"

	router, err := events.NewRouter()
	require.NoError(t, err)
	t.Cleanup(func() { _ = router.Close() })
	cfg, err := config.Load("test")
	require.NoError(t, err)
	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	bus := events.New(sqlDB)
	mod := outbound.New(bus, nil, nil, tracking.New("test-secret", "http://local"))
	queue, err := jobs.NewClient(sqlDB, client, mod, cipher, bus, nil, nil, nil, registry.Default(), cfg.AppURL, cfg.OutboundAllow, nil, jobs.Retention{})
	require.NoError(t, err)
	reader, err := river.NewClient(riverdatabasesql.New(sqlDB), &river.Config{})
	require.NoError(t, err)
	require.NoError(t, events.RegisterSubscribers(router, sqlDB, client, queue, queue, queue, nil, testhelper.PrivateOutbox()))

	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go func() { _ = router.Run(runCtx) }()
	<-router.Running()

	require.NoError(t, events.New(sqlDB).WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(ctx, &events.EmailDelivery{
			Action: events.NameEmailBounced, WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactErasableID,
			Email: dest, BounceKind: events.BounceKindPermanent, DedupID: "registered-bounce-1",
		})
	}))

	require.Eventually(t, func() bool {
		persisted, _ := client.Event.Query().Where(event.SubjectID(dest)).Exist(ctx)
		suppressed, _ := client.Suppression.Query().Where(suppression.Destination(dest)).Exist(ctx)
		return persisted && suppressed && queued(ctx, t, reader, jobkind.EvaluateTrigger.Kind) >= 1 && queued(ctx, t, reader, jobkind.DeliverWebhook.Kind) >= 1
	}, 15*time.Second, 50*time.Millisecond, "persist, suppression, automations and webhooks must all see the event")
	// Delivery is at-least-once: a serialization failure while acking redelivers the
	// message, so a consumer may see it more than once, never a different event.
	deliveries, err := reader.JobList(ctx, river.NewJobListParams().Kinds(jobkind.DeliverWebhook.Kind).First(100))
	require.NoError(t, err)
	for _, j := range deliveries.Jobs {
		var args jobs.DeliverWebhookArgs
		require.NoError(t, json.Unmarshal(j.EncodedArgs, &args))
		assert.Equal(t, events.NameEmailBounced, args.EventName)
	}
}

// queued counts the jobs of kind waiting in the real queue.
func queued(ctx context.Context, t *testing.T, reader *river.Client[*sql.Tx], kind string) int {
	t.Helper()
	res, err := reader.JobList(ctx, river.NewJobListParams().Kinds(kind).First(100))
	require.NoError(t, err)
	return len(res.Jobs)
}
