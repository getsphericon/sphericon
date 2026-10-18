package telemetryschema_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	telemetryapi "github.com/getsphericon/sphericon/gen/telemetry"
	"github.com/getsphericon/sphericon/internal/telemetryschema"
)

func sampleReport() telemetryapi.TelemetryReport {
	return telemetryapi.TelemetryReport{
		SchemaVersion:      1,
		InstanceId:         "6f1c1d0e-8a5b-4f43-9a52-2b1f2f6a7c10",
		AppVersion:         "1.4.2",
		Edition:            telemetryapi.TelemetryEditionCore,
		Os:                 "linux",
		Arch:               "arm64",
		DeployShape:        telemetryapi.TelemetryDeployShapeDocker,
		PostgresMajor:      17,
		Locale:             telemetryapi.TelemetryLocaleEn,
		InstanceAgeDays:    42,
		Workspaces:         telemetryapi.OrderOfMagnitude19,
		Contacts:           telemetryapi.OrderOfMagnitude100999,
		EmailsSent30d:      telemetryapi.OrderOfMagnitude10009999,
		UsedBroadcasts30d:  true,
		UsedTracker30d:     true,
		WebhooksConfigured: false,
		IntegrationKinds:   []telemetryapi.TelemetryIntegrationKind{telemetryapi.TelemetryIntegrationKindSMTP},
	}
}

func marshalFields(t *testing.T, report telemetryapi.TelemetryReport) map[string]any {
	t.Helper()
	body, err := json.Marshal(report)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(body, &fields))
	return fields
}

func validate(t *testing.T, fields map[string]any) error {
	t.Helper()
	body, err := json.Marshal(fields)
	require.NoError(t, err)
	return telemetryschema.Validate(body)
}

func TestSampleReportMatchesPublishedSchema(t *testing.T) {
	body, err := json.Marshal(sampleReport())
	require.NoError(t, err)

	require.NoError(t, telemetryschema.Validate(body))
}

func TestReportWithExactCountIsRejected(t *testing.T) {
	fields := marshalFields(t, sampleReport())
	fields["contacts"] = "137"

	require.Error(t, validate(t, fields))
}

func TestReportWithUnlistedFieldIsRejected(t *testing.T) {
	fields := marshalFields(t, sampleReport())
	fields["workspaceName"] = "Acme"

	require.Error(t, validate(t, fields))
}

func TestReportMissingFieldIsRejected(t *testing.T) {
	fields := marshalFields(t, sampleReport())
	delete(fields, "instanceId")

	require.Error(t, validate(t, fields))
}

func TestNonJSONIsRejected(t *testing.T) {
	require.Error(t, telemetryschema.Validate([]byte("not json")))
}
