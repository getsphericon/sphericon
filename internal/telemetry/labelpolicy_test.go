package telemetry_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/messaging/registry"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/telemetry"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// forbiddenLabel matches label names that identify a tenant or a person (ADR 0018):
// workspace id or slug, contact, email, recipient. Span attributes are out of scope.
var forbiddenLabel = regexp.MustCompile(`(?i)workspace|slug|contact|email|recipient`)

// The metric label policy (ADR 0018) is enforced against the real exposition of the
// metrics listener after real instrumented work ran: a domain event handled by the
// watermill router (carrying a Workspace id and a Contact email) and a job run by the
// river runtime. Any tenant-bearing label or label value would show up here.
func TestMetricsExpositionCarriesNoTenantLabels(t *testing.T) {
	ctx := context.Background()
	cfg, err := config.Load("test")
	require.NoError(t, err)

	// Global providers must be installed before the instrumented components are built.
	testhelper.InstallOtel(t, func(ctx context.Context) (func(context.Context) error, error) {
		return telemetry.Setup(ctx, &config.Config{OtelServiceName: "sphericon-test"}, "test", telemetry.BuildInfo{})
	})

	srv := telemetry.NewMetricsServer("127.0.0.1:0")
	require.NoError(t, srv.Listen(t.Context()))
	go func() { _ = srv.Serve() }()
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	env := testhelper.Setup(t)
	workspace := env.DB.Workspace.GetX(ctx, fixtures.AcmeID)
	contactEmail := fixtures.ContactAliceEmail

	runHandledDomainEvent(t, cfg, srv.Addr(), contactEmail)
	runJob(t, env, cfg, srv.Addr(), contactEmail)

	code, body := testhelper.HTTPGet(t, "http://"+srv.Addr()+"/metrics")
	require.Equal(t, http.StatusOK, code)

	// Proof that the instrumented paths ran: their metrics are in the exposition.
	require.Contains(t, body, "watermill_messages_processed", "domain-event handler metrics missing")
	require.Contains(t, body, "river_", "job metrics missing")

	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(body))
	require.NoError(t, err)
	require.NotEmpty(t, families)
	for name, family := range families {
		for _, m := range family.GetMetric() {
			for _, label := range m.GetLabel() {
				assert.NotRegexp(t, forbiddenLabel, label.GetName(), "forbidden label name on %s", name)
				for _, secret := range []string{workspace.Slug, contactEmail} {
					assert.NotContains(t, label.GetValue(), secret, "tenant value in label %s of %s", label.GetName(), name)
				}
			}
		}
	}
}

// runHandledDomainEvent publishes a tenant-bearing event through the outbox and waits
// until the real router (with the OTel middleware) has handled it.
func runHandledDomainEvent(t *testing.T, cfg *config.Config, srvAddr, contactEmail string) {
	t.Helper()
	ctx := context.Background()
	sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, events.InitSchema(ctx, sqlDB))
	t.Cleanup(func() { _ = testhelper.PurgeOutbox(context.WithoutCancel(ctx), sqlDB) })

	router, err := events.NewRouter()
	require.NoError(t, err)
	t.Cleanup(func() { _ = router.Close() })
	sub, err := events.NewSubscriber(sqlDB, "label-policy")
	require.NoError(t, err)
	handled := make(chan struct{}, 1)
	router.AddConsumerHandler("label_policy", events.TopicDomainEvents, sub, func(msg *message.Message) error {
		var env events.Envelope
		if err := json.Unmarshal(msg.Payload, &env); err != nil {
			return err
		}
		select {
		case handled <- struct{}{}:
		default:
		}
		return nil
	})
	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go func() { _ = router.Run(runCtx) }()
	<-router.Running()

	require.NoError(t, events.New(sqlDB).WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(ctx, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Email: contactEmail})
	}))
	select {
	case <-handled:
	case <-time.After(10 * time.Second):
		t.Fatal("domain event was not handled within 10s")
	}
	// The middleware records its metrics right after the handler returns.
	require.Eventually(t, func() bool { return exposes(t, srvAddr, "watermill_messages_processed") }, 10*time.Second, 50*time.Millisecond)
}

// runJob runs a real river runtime and waits for a job to be worked.
func runJob(t *testing.T, env *testhelper.TestEnv, cfg *config.Config, srvAddr, contactEmail string) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, jobs.Migrate(ctx, pool))
	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	mod := outbound.New(env.Bus, nil, tracking.New("test-secret", "http://local"))
	client, err := jobs.NewClient(pool, env.DB, env.SQLDB, mod, cipher, env.SystemMail, nil, registry.Default(), cfg.AppURL, jobs.Retention{OutboxFloor: cfg.OutboxFloor, Events: cfg.EventsRetention})
	require.NoError(t, err)
	admin, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	purge := func() {
		_, _ = admin.JobDeleteMany(context.Background(), river.NewJobDeleteManyParams().Queues(river.QueueDefault, jobs.QueueBroadcasts, jobs.QueueWebhooks))
	}
	purge()
	t.Cleanup(purge)
	require.NoError(t, client.Start(ctx))
	t.Cleanup(func() { _ = client.Stop(context.Background()) })

	require.NoError(t, client.EnqueueWelcome(ctx, contactEmail, "Policy"))
	require.Eventually(t, func() bool {
		for _, m := range env.SystemMail.Messages() {
			if m.To == contactEmail {
				return true
			}
		}
		return false
	}, 10*time.Second, 50*time.Millisecond)
	require.Eventually(t, func() bool { return exposes(t, srvAddr, "river_") }, 10*time.Second, 50*time.Millisecond)
}

// exposes reports whether the live exposition at addr contains substr.
func exposes(t *testing.T, addr, substr string) bool {
	t.Helper()
	code, body, err := testhelper.TryHTTPGet(t.Context(), "http://"+addr+"/metrics")
	return err == nil && code == http.StatusOK && strings.Contains(body, substr)
}
