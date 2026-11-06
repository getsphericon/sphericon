package telemetry_test

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/kernel/mailjobs"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging/registry"
	"github.com/getsphericon/sphericon/internal/infra/telemetry"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// forbiddenLabel matches label names that identify a tenant or a person (ADR 0018):
// workspace id, contact, email, recipient. Span attributes are out of scope.
var forbiddenLabel = regexp.MustCompile(`(?i)workspace|contact|email|recipient`)

// The metric label policy (ADR 0018) is enforced against the real exposition of the
// metrics listener after real instrumented work ran: jobs worked by the river runtime, a
// domain event's job among them (its envelope carrying a Workspace id and a Contact email).
// Any tenant-bearing label or label value would show up here.
func TestMetricsExpositionCarriesNoTenantLabels(t *testing.T) {
	ctx := context.Background()

	// Global providers must be installed before the instrumented components are built.
	testhelper.InstallOtel(t, func(ctx context.Context) (func(context.Context) error, error) {
		return telemetry.Setup(ctx, &config.Config{OtelServiceName: "sphericon-test"}, "test", telemetry.BuildInfo{})
	})

	srv := telemetry.NewMetricsServer("127.0.0.1:0")
	require.NoError(t, srv.Listen(t.Context()))
	go func() { cleanup.Served("srv", srv.Serve()) }()
	t.Cleanup(func() { cleanup.Report("shut srv down", srv.Shutdown(context.Background())) })

	env := testhelper.Setup(t)
	// Loaded after Setup: it points SPHERICON_DATABASE_URL at this process's database, where river's
	// tables are, and the job runtime below opens its own pool on it.
	cfg, err := config.Load("test")
	require.NoError(t, err)
	workspace := env.DB.Workspace.GetX(ctx, fixtures.AcmeID)
	contactEmail := fixtures.ContactAliceEmail

	runJob(t, env, cfg, srv.Addr(), contactEmail)

	code, body := testhelper.HTTPGet(t, "http://"+srv.Addr()+"/metrics")
	require.Equal(t, http.StatusOK, code)

	// Proof that the instrumented paths ran: their metrics are in the exposition.
	require.Contains(t, body, "river_", "job metrics missing")

	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(body))
	require.NoError(t, err)
	require.NotEmpty(t, families)
	for name, family := range families {
		for _, m := range family.GetMetric() {
			for _, label := range m.GetLabel() {
				assert.NotRegexp(t, forbiddenLabel, label.GetName(), "forbidden label name on %s", name)
				for _, secret := range []string{workspace.PublicID, contactEmail} {
					assert.NotContains(t, label.GetValue(), secret, "tenant value in label %s of %s", label.GetName(), name)
				}
			}
		}
	}
}

// runJob runs a real river runtime and waits for a job to be worked.
func runJob(t *testing.T, env *testhelper.TestEnv, cfg *config.Config, srvAddr, contactEmail string) {
	t.Helper()
	ctx := context.Background()
	pool, err := db.OpenRiverDB(cfg.DatabaseURL, cfg.DBPool)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(pool, "pool") })
	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	mod := outbound.New(env.Bus, env.Holds, nil, tracking.New("test-secret", "http://local"))
	client, err := jobs.NewClient(riverdatabasesql.New(pool), jobs.Deps{
		Ent: env.DB, Bus: env.Bus, Outbound: mod, Cipher: cipher, SystemSender: env.SystemMail, Catalog: registry.Default(),
		AppURL: cfg.AppURL, OutboundAllow: cfg.OutboundAllow,
		Retention: jobs.Retention{Events: cfg.EventsRetention, UnverifiedUsers: cfg.UnverifiedUserRetention},
	})
	require.NoError(t, err)
	admin, err := river.NewClient(riverdatabasesql.New(pool), &river.Config{})
	require.NoError(t, err)
	purge := func() {
		_, err := admin.JobDeleteMany(context.Background(), river.NewJobDeleteManyParams().Queues(river.QueueDefault, broadcasts.Queue, jobs.QueueWebhooks, events.QueueEvents))
		require.NoError(t, err)
	}
	purge()
	t.Cleanup(purge)
	require.NoError(t, client.Start(ctx))
	t.Cleanup(func() { cleanup.Report("stop the job client", client.Stop(context.Background())) })

	queue, err := jobs.NewQueue(pool, nil)
	require.NoError(t, err)
	require.NoError(t, queue.Enqueue(ctx, mailjobs.Welcome(contactEmail, "Policy")))
	require.Eventually(t, func() bool {
		msgs := env.SystemMail.Messages()
		for i := range msgs {
			if msgs[i].To == contactEmail {
				return true
			}
		}
		return false
	}, 10*time.Second, 50*time.Millisecond)
	require.Eventually(t, func() bool { return exposes(t, srvAddr, "river_") }, 10*time.Second, 50*time.Millisecond)

	// A domain event's job, which carries the event's Workspace and Contact address in its args. A
	// soft bounce is one the suppression consumer turns down without a write, so nothing commits to
	// this process's database.
	data, err := json.Marshal(&events.EmailDelivery{
		Action: events.NameEmailBounced, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID,
		Email: contactEmail, BounceKind: events.BounceKindSoft,
	})
	require.NoError(t, err)
	_, err = admin.Insert(ctx, jobs.SuppressEventArgs{
		ID: "01JLABELPOLICY", Name: events.NameEmailBounced, Version: 1, WorkspaceID: fixtures.AcmeID, Data: data,
	}, nil)
	require.NoError(t, err)
	worked := `kind="` + jobs.SuppressEventArgs{}.Kind() + `"`
	require.Eventually(t, func() bool { return exposes(t, srvAddr, worked) }, 10*time.Second, 50*time.Millisecond, "the event job was worked")
	// The fetch metrics come from otelriver's hook, which only a plugin installs.
	require.Eventually(t, func() bool { return exposes(t, srvAddr, "river_job_get_available") }, 10*time.Second, 50*time.Millisecond)
}

// exposes reports whether the live exposition at addr contains substr.
func exposes(t *testing.T, addr, substr string) bool {
	t.Helper()
	code, body, err := testhelper.TryHTTPGet(t.Context(), "http://"+addr+"/metrics")
	return err == nil && code == http.StatusOK && strings.Contains(body, substr)
}
