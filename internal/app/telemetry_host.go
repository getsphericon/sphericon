package app

import (
	"runtime"
	"time"

	"github.com/getsphericon/sphericon/config"
	telemetryapi "github.com/getsphericon/sphericon/gen/telemetry"
	"github.com/getsphericon/sphericon/internal/domain/instance/instancestates"
	"github.com/getsphericon/sphericon/internal/domain/instance/upgrade"
)

// telemetryHost reads what the Telemetry report says about this process: the version it was
// built as, the edition its license key switches on (a bad key fails boot, so a key that is set
// is a valid one, and a lapsed one, whose expiry the caller passes, runs the core: ADR 0035),
// where it runs and the configured locale.
func telemetryHost(cfg *config.Config, version string, licenseExpiredAt time.Time) instancestates.TelemetryHost {
	edition := telemetryapi.TelemetryEditionCore
	if cfg.LicenseKey != "" && licenseExpiredAt.IsZero() {
		edition = telemetryapi.TelemetryEditionEe
	}
	return instancestates.TelemetryHost{
		AppVersion:  version,
		Edition:     edition,
		DeployShape: deployShapeOf(upgrade.DetectContainer()),
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		Locale:      telemetryapi.TelemetryLocaleEn,
	}
}

// deployShapeOf maps where upgrade.DetectContainer found the Instance runs to the schema's deploy
// shape.
func deployShapeOf(container upgrade.Container) telemetryapi.TelemetryDeployShape {
	switch container {
	case upgrade.Helm:
		return telemetryapi.TelemetryDeployShapeKubernetes
	case upgrade.Docker:
		return telemetryapi.TelemetryDeployShapeContainer
	case upgrade.BareHost:
		return telemetryapi.TelemetryDeployShapeBinary
	}
	return telemetryapi.TelemetryDeployShapeUnknown
}
