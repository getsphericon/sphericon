package instancestates_test

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/integration"
	telemetryapi "github.com/getsphericon/sphericon/gen/telemetry"
	"github.com/getsphericon/sphericon/internal/domain/instance/instancestates"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/telemetryschema"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var testHost = instancestates.TelemetryHost{
	AppVersion:  "1.4.2",
	Edition:     telemetryapi.TelemetryEditionCore,
	DeployShape: telemetryapi.TelemetryDeployShapeContainer,
	OS:          "linux",
	Arch:        "arm64",
	Locale:      telemetryapi.TelemetryLocaleEn,
}

func buildReport(t *testing.T, env *testhelper.TestEnv, now time.Time) telemetryapi.TelemetryReport {
	t.Helper()
	mod := instancestates.New(env.DB, instancestates.Environment{}, func() time.Time { return now }, nil, nil, testHost, nil, slog.Default())
	report, err := mod.TelemetryReport(t.Context())
	require.NoError(t, err)
	return report
}

func TestReportFromFixturesIsValidAndBucketed(t *testing.T) {
	env := testhelper.Setup(t)

	report := buildReport(t, env, time.Now())

	body, err := json.Marshal(report)
	require.NoError(t, err)
	require.NoError(t, telemetryschema.Validate(body))
	assert.Equal(t, "1.4.2", report.AppVersion)
	assert.Equal(t, telemetryapi.TelemetryDeployShapeContainer, report.DeployShape)
	assert.Equal(t, telemetryapi.TelemetryLocaleEn, report.Locale)
	assert.Equal(t, telemetryapi.OrderOfMagnitude1099, report.Workspaces)
	assert.Equal(t, telemetryapi.OrderOfMagnitude100999, report.Contacts)
	assert.Equal(t, telemetryapi.OrderOfMagnitude100999, report.EmailsSent30d)
	assert.True(t, report.UsedBroadcasts30d)
	assert.True(t, report.AutomationsActive)
	assert.True(t, report.UsedTracker30d)
	assert.True(t, report.UsedExternalApi30d)
	assert.True(t, report.WebhooksConfigured)
	assert.ElementsMatch(t, []telemetryapi.TelemetryIntegrationKind{
		telemetryapi.TelemetryIntegrationKindSMTP, telemetryapi.TelemetryIntegrationKindSes,
	}, report.IntegrationKinds)
	assert.GreaterOrEqual(t, int(report.PostgresMajor), 12)
}

func TestReportNamesNeitherAWorkspaceNorAPerson(t *testing.T) {
	env := testhelper.Setup(t)

	body, err := json.Marshal(buildReport(t, env, time.Now()))
	require.NoError(t, err)

	for _, secret := range []string{fixtures.AcmePublicID, fixtures.OwnerJohnEmail, "http", "@"} {
		assert.NotContains(t, string(body), secret)
	}
}

func TestReportActivityFlagsFallAwayWhenNothingHappenedInThirtyDays(t *testing.T) {
	env := testhelper.Setup(t)

	report := buildReport(t, env, time.Now().AddDate(1, 0, 0))

	assert.Equal(t, telemetryapi.OrderOfMagnitude0, report.EmailsSent30d)
	assert.False(t, report.UsedBroadcasts30d)
	assert.False(t, report.UsedTracker30d)
	assert.False(t, report.UsedExternalApi30d)
}

func TestReportAgeCountsWholeDaysSinceTheInstanceFirstStarted(t *testing.T) {
	env := testhelper.Setup(t)
	row, err := instancestates.New(env.DB, instancestates.Environment{}, time.Now, nil, nil, testHost, nil, slog.Default()).Ensure(t.Context())
	require.NoError(t, err)

	report := buildReport(t, env, row.CreatedAt.Add(42*24*time.Hour+3*time.Hour))

	assert.Equal(t, int32(42), report.InstanceAgeDays)
	assert.Equal(t, row.InstanceID, report.InstanceId)
}

func TestReportEditionAndDeployShapeAreWhatTheCallerSays(t *testing.T) {
	env := testhelper.Setup(t)
	host := testHost
	host.Edition = telemetryapi.TelemetryEditionEe
	host.DeployShape = telemetryapi.TelemetryDeployShapeKubernetes

	report, err := instancestates.New(env.DB, instancestates.Environment{}, time.Now, nil, nil, host, nil, slog.Default()).TelemetryReport(t.Context())
	require.NoError(t, err)

	assert.Equal(t, telemetryapi.TelemetryEditionEe, report.Edition)
	assert.Equal(t, telemetryapi.TelemetryDeployShapeKubernetes, report.DeployShape)
}

// The providers in use are those of kept Integrations (ADR 0099): once every SES Integration of
// the Instance is deleted, SES is no longer listed.
func TestReportProvidersComeFromKeptIntegrationsOnly(t *testing.T) {
	env := testhelper.Setup(t)
	ses, err := env.DB.Integration.Query().
		Where(integration.ProviderEQ(providers.SES), integration.DiscardedAtIsNil()).
		All(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, ses)
	for _, i := range ses {
		require.NoError(t, env.DB.Scoped(i.WorkspaceID).Integration().Discard(t.Context(), i.ID, time.Now()))
	}

	report := buildReport(t, env, time.Now())

	assert.Equal(t, []telemetryapi.TelemetryIntegrationKind{telemetryapi.TelemetryIntegrationKindSMTP}, report.IntegrationKinds)
}
