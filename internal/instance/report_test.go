package instance_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	telemetryapi "github.com/getsphericon/sphericon/gen/telemetry"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/instance"
	"github.com/getsphericon/sphericon/internal/telemetryschema"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var testHost = instance.Host{
	AppVersion:  "1.4.2",
	Edition:     telemetryapi.TelemetryEditionCore,
	DeployShape: telemetryapi.TelemetryDeployShapeContainer,
	OS:          "linux",
	Arch:        "arm64",
	Locale:      telemetryapi.TelemetryLocaleEn,
}

func buildReport(t *testing.T, env *testhelper.TestEnv, now time.Time) telemetryapi.TelemetryReport {
	t.Helper()
	report, err := instance.BuildReport(t.Context(), env.DB, testHost, now)
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
	row, err := instance.New(env.DB, instance.Environment{}, testHost, nil).Ensure(t.Context())
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

	report, err := instance.BuildReport(t.Context(), env.DB, host, time.Now())
	require.NoError(t, err)

	assert.Equal(t, telemetryapi.TelemetryEditionEe, report.Edition)
	assert.Equal(t, telemetryapi.TelemetryDeployShapeKubernetes, report.DeployShape)
}
