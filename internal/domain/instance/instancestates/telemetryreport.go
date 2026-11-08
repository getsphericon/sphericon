package instancestates

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	telemetryapi "github.com/getsphericon/sphericon/gen/telemetry"
	"github.com/getsphericon/sphericon/internal/kernel/buildinfo"
)

const (
	// reportInterval is how often an Instance reports: once a day.
	reportInterval = 24 * time.Hour
	// reportTimeout bounds one send; there is no retry beyond the next cycle.
	reportTimeout = 5 * time.Second
)

// reportDue is whether a report is due at now. The first one goes out once the Instance's
// offset (ReportOffset) has passed since its notice was acknowledged (never at the first tick).
// After that the report goes out in the day's slot, midnight UTC plus the offset, and at least
// half a day after the last one, so the time of day does not drift and a first report sent late
// in the day is not followed by a second a few hours later.
func reportDue(row *ent.InstanceState, now time.Time) bool {
	offset := ReportOffset(row.InstanceID)
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

// TelemetryReport is the Telemetry report as the Instance would send it now, whether or not
// sending is on: the same builder the daily send uses, which a Staff member sees as a preview.
func (m *Module) TelemetryReport(ctx context.Context) (telemetryapi.TelemetryReport, error) {
	return m.buildReport(ctx, m.now())
}

// SendTelemetryReport sends the Telemetry report when every gate lets it through: never in a
// development environment, from a build that is not a release or without a receiver, and
// otherwise only while the effective setting is on, a Staff member acknowledged the notice in
// the console (ADR 0063: an Instance without a Staff member sends nothing) and a report is due.
// A failed send is logged at debug level and left for the next cycle; only a failure to read or
// store the state is an error.
func (m *Module) SendTelemetryReport(ctx context.Context) error {
	if m.receiver == nil || m.env.Dev || !buildinfo.IsRelease(m.host.AppVersion) {
		return nil
	}
	row, err := m.Ensure(ctx)
	if err != nil {
		return err
	}
	if !Effective(m.env, row.TelemetryEnabled).Enabled || row.NoticeAcknowledgedAt == nil {
		return nil
	}
	now := m.now()
	if !reportDue(row, now) {
		return nil
	}
	report, err := m.buildReport(ctx, now)
	if err != nil {
		return err
	}
	sendCtx, cancel := context.WithTimeout(ctx, reportTimeout)
	defer cancel()
	if err := m.receiver.TelemetryReportsSubmit(sendCtx, &report); err != nil {
		m.log.DebugContext(ctx, "telemetry report not sent", "err", err)
		return nil
	}
	return m.markReportSent(ctx, now)
}
