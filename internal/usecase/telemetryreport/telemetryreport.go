// Package telemetryreport is the use case of the Telemetry report (ADR 0032, ADR 0102): it builds
// the report from the Instance's figures and what the running binary says of itself, shows it to a
// Staff member as a preview, and sends it once a day to the receiver of the Telemetry contract,
// whose client it is (gen/telemetry). Whether a report is sent and when the last one went out are
// the Instance state's (internal/domain/instance/instancestates), which it reads and records through that module.
// It takes the Instance state module rather than an *ent.Scoped: the report belongs to no
// Workspace.
package telemetryreport

import (
	"context"
	"log/slog"
	"time"

	"github.com/getsphericon/sphericon/ent"
	telemetryapi "github.com/getsphericon/sphericon/gen/telemetry"
	"github.com/getsphericon/sphericon/internal/domain/instance/instancestates"
	"github.com/getsphericon/sphericon/internal/kernel/buildinfo"
)

const (
	// reportInterval is how often an Instance reports: once a day.
	reportInterval = 24 * time.Hour
	// reportTimeout bounds one send; there is no retry beyond the next cycle.
	reportTimeout = 5 * time.Second
)

// reportDue is whether a report is due at now. The first one goes out once the Instance's
// offset (instancestates.ReportOffset) has passed since its notice was acknowledged (never at
// the first tick). After that the report goes out in the day's slot, midnight UTC plus the
// offset, and at least half a day after the last one, so the time of day does not drift and a
// first report sent late in the day is not followed by a second a few hours later.
func reportDue(row *ent.InstanceState, now time.Time) bool {
	offset := instancestates.ReportOffset(row.InstanceID)
	last := row.LastReportSentAt
	if last == nil {
		return now.Sub(*row.NoticeAcknowledgedAt) >= offset
	}
	slot := now.UTC().Truncate(reportInterval).Add(offset)
	if slot.After(now) {
		slot = slot.Add(-reportInterval)
	}
	return last.Before(slot) && now.Sub(*last) >= reportInterval/2
}

// UseCase builds and sends the Telemetry report. It sends nothing in a development environment
// or from a build that is not a release, and otherwise only while the effective setting is on, a
// Staff member acknowledged the notice in the console (ADR 0063: an Instance without a Staff member
// sends nothing) and a report is due.
type UseCase struct {
	states  *instancestates.Module
	host    Host
	now     func() time.Time
	client  *telemetryapi.Client
	timeout time.Duration
	log     *slog.Logger
}

// New builds the use case over the Instance state, with host what the report says about the
// running binary, client the client of the receiver (built by the wiring from
// config.TelemetryReceiverURL) and now the Instance clock. A development environment (the
// Instance state's Environment.Dev) never sends, so a test opts in by building its config as
// production.
func New(states *instancestates.Module, host Host, client *telemetryapi.Client, now func() time.Time) *UseCase {
	return &UseCase{states: states, host: host, now: now, client: client, timeout: reportTimeout, log: slog.Default()}
}

// Report is the Telemetry report as the Instance would send it now, whether or not sending
// is on: the same builder the daily sender uses.
func (u *UseCase) Report(ctx context.Context) (telemetryapi.TelemetryReport, error) {
	return build(ctx, u.states, u.host, u.now())
}

// Send sends the report when every gate lets it through. A failed send is logged at debug
// level and left for the next cycle; only a failure to read or store the state is an error.
func (u *UseCase) Send(ctx context.Context) error {
	env := u.states.Environment()
	if env.Dev || !buildinfo.IsRelease(u.host.AppVersion) {
		return nil
	}
	row, err := u.states.Ensure(ctx)
	if err != nil {
		return err
	}
	if !instancestates.Effective(env, row.TelemetryEnabled).Enabled || row.NoticeAcknowledgedAt == nil {
		return nil
	}
	now := u.now()
	if !reportDue(row, now) {
		return nil
	}
	report, err := u.Report(ctx)
	if err != nil {
		return err
	}
	sendCtx, cancel := context.WithTimeout(ctx, u.timeout)
	defer cancel()
	if err := u.client.TelemetryReportsSubmit(sendCtx, &report); err != nil {
		u.log.DebugContext(ctx, "telemetry report not sent", "err", err)
		return nil
	}
	return u.states.MarkReportSent(ctx, now)
}
