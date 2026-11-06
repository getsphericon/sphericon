package telemetryreport

import (
	"context"
	"runtime"
	"slices"
	"time"

	"github.com/getsphericon/sphericon/config"
	telemetryapi "github.com/getsphericon/sphericon/gen/telemetry"
	"github.com/getsphericon/sphericon/internal/instancestates"
	"github.com/getsphericon/sphericon/internal/kernel/narrow"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/upgrade"
)

// schemaVersion is the version of the Telemetry report schema the builder implements
// (typespec/telemetry). A change to the schema bumps it in the same commit.
const schemaVersion = 1

// reportWindow is the look-back of every "in the last 30 days" figure.
const reportWindow = 30 * 24 * time.Hour

// Host is what the report says about the running binary and its surroundings. The builder
// never reads the process (build info, environment, files): the caller hands these in, so a
// test builds the same report every time.
type Host struct {
	AppVersion  string
	Edition     telemetryapi.TelemetryEdition
	DeployShape telemetryapi.TelemetryDeployShape
	OS          string
	Arch        string
	Locale      telemetryapi.TelemetryLocale
}

// HostOf reads the Host of this process: the version it was built as, the edition its
// license key switches on (a bad key fails boot, so a key that is set is a valid one, and a
// lapsed one, whose expiry the caller passes, runs the core: ADR 0035), where it runs and the
// configured locale.
func HostOf(cfg *config.Config, version string, licenseExpiredAt time.Time) Host {
	edition := telemetryapi.TelemetryEditionCore
	if cfg.LicenseKey != "" && licenseExpiredAt.IsZero() {
		edition = telemetryapi.TelemetryEditionEe
	}
	return Host{
		AppVersion:  version,
		Edition:     edition,
		DeployShape: DeployShapeOf(upgrade.DetectContainer()),
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		Locale:      telemetryapi.TelemetryLocaleEn,
	}
}

// DeployShapeOf maps what upgrade.DetectContainer found ("helm" on Kubernetes, "docker" in
// a container, "" otherwise) to the schema's deploy shape.
func DeployShapeOf(container string) telemetryapi.TelemetryDeployShape {
	switch container {
	case "helm":
		return telemetryapi.TelemetryDeployShapeKubernetes
	case "docker":
		return telemetryapi.TelemetryDeployShapeContainer
	case "":
		return telemetryapi.TelemetryDeployShapeBinary
	}
	return telemetryapi.TelemetryDeployShapeUnknown
}

// Build assembles the Telemetry report (ADR 0032) from the Instance's Figures, read from core
// tables only. It is the one builder, the preview and the daily sender both call it. Counts leave it
// only as orders of magnitude, so an exact number never does, and it reads nothing that names a
// Workspace or a person.
func Build(ctx context.Context, states *instancestates.Module, host Host, now time.Time) (telemetryapi.TelemetryReport, error) {
	f, err := states.Figures(ctx, now.Add(-reportWindow))
	if err != nil {
		return telemetryapi.TelemetryReport{}, err
	}
	row := f.State
	kinds := make([]telemetryapi.TelemetryIntegrationKind, 0, len(f.Providers))
	for _, p := range f.Providers {
		switch p {
		case providers.SMTP:
			kinds = append(kinds, telemetryapi.TelemetryIntegrationKindSMTP)
		case providers.SES:
			kinds = append(kinds, telemetryapi.TelemetryIntegrationKindSes)
		case providers.Inbox:
			// A Test inbox sends nothing: it is not a sending Integration in use.
		case providers.Telegram, providers.Twilio:
			// The Telemetry report lists email providers; a Telegram or Twilio kind is a change of its
			// schema.
		}
	}
	slices.Sort(kinds)

	return telemetryapi.TelemetryReport{
		SchemaVersion:      schemaVersion,
		InstanceId:         row.InstanceID,
		AppVersion:         host.AppVersion,
		Edition:            host.Edition,
		Os:                 host.OS,
		Arch:               host.Arch,
		DeployShape:        host.DeployShape,
		PostgresMajor:      narrow.Int32(f.PostgresMajor),
		Locale:             host.Locale,
		InstanceAgeDays:    max(0, narrow.Int32(now.Sub(row.CreatedAt)/(24*time.Hour))),
		Workspaces:         bucket(f.Workspaces),
		Contacts:           bucket(f.Contacts),
		EmailsSent30d:      bucket(f.EmailsSent),
		UsedBroadcasts30d:  f.UsedBroadcasts,
		AutomationsActive:  f.AutomationsActive,
		UsedTracker30d:     f.UsedTracker,
		UsedExternalApi30d: f.UsedExternalAPI,
		WebhooksConfigured: f.WebhooksConfigured,
		IntegrationKinds:   kinds,
	}, nil
}

// bucket rounds a count to its order of magnitude: the one place an exact number is dropped.
func bucket(n int) telemetryapi.OrderOfMagnitude {
	switch {
	case n <= 0:
		return telemetryapi.OrderOfMagnitude0
	case n < 10:
		return telemetryapi.OrderOfMagnitude19
	case n < 100:
		return telemetryapi.OrderOfMagnitude1099
	case n < 1_000:
		return telemetryapi.OrderOfMagnitude100999
	case n < 10_000:
		return telemetryapi.OrderOfMagnitude10009999
	case n < 100_000:
		return telemetryapi.OrderOfMagnitude1000099999
	case n < 1_000_000:
		return telemetryapi.OrderOfMagnitude100000999999
	}
	return telemetryapi.OrderOfMagnitude1000000
}
