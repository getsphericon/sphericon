package instance

import (
	"context"
	"hash/fnv"
	"time"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	telemetryapi "github.com/getsphericon/sphericon/gen/telemetry"
	"github.com/getsphericon/sphericon/internal/buildinfo"
)

const (
	// reportInterval is how often an Instance reports: once a day.
	reportInterval = 24 * time.Hour
	// reportSpread is the length of the window an Instance's report time is spread over,
	// so Instances that started together do not all report in the same minute.
	reportSpread = 6 * time.Hour
	// reportTimeout bounds one send; there is no retry beyond the next cycle.
	reportTimeout = 5 * time.Second
)

// ReportOffset is how far into the day (UTC) the Instance reports: a fixed 0 to 6 hours
// derived from its Instance id, so it is the same every day and after every restart.
func ReportOffset(instanceID string) time.Duration {
	h := fnv.New64a()
	_, _ = h.Write([]byte(instanceID))
	return (time.Duration(h.Sum64() % uint64(reportSpread))).Truncate(time.Second)
}

// reportDue is whether a report is due at now. The first one goes out once the Instance's
// offset has passed since its notice was acknowledged (never at the first tick). After that
// the report goes out in the day's slot, midnight UTC plus the offset, and at least half a
// day after the last one, so the time of day does not drift and a first report sent late in
// the day is not followed by a second a few hours later.
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

// Sender sends the daily Telemetry report (ADR 0032). It sends nothing in a development
// environment or from a build that is not a release, and otherwise only while the effective
// setting is on, a Staff member acknowledged the notice in the console (ADR 0063: an Instance
// without a Staff member sends nothing) and a report is due.
type Sender struct {
	mod     *Module
	client  *telemetryapi.Client
	timeout time.Duration
}

// SenderOption tunes a Sender.
type SenderOption func(*Sender)

// NewSender builds the sender over the receiver address in cfg. A development environment
// (the module's Environment.Dev) never sends, so a test opts in by building its config as
// production.
func NewSender(mod *Module, cfg *config.Config, opts ...SenderOption) (*Sender, error) {
	client, err := telemetryapi.NewClient(cfg.TelemetryReceiverURL)
	if err != nil {
		return nil, err
	}
	s := &Sender{mod: mod, client: client, timeout: reportTimeout}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// Run sends the report when every gate lets it through. A failed send is logged at debug
// level and left for the next cycle; only a failure to read or store the state is an error.
func (s *Sender) Run(ctx context.Context) error {
	if s.mod.env.Dev || !buildinfo.IsRelease(s.mod.host.AppVersion) {
		return nil
	}
	row, err := s.mod.Ensure(ctx)
	if err != nil {
		return err
	}
	if !Effective(s.mod.env, row.TelemetryEnabled).Enabled || row.NoticeAcknowledgedAt == nil {
		return nil
	}
	now := s.mod.now()
	if !reportDue(row, now) {
		return nil
	}
	report, err := s.mod.Report(ctx)
	if err != nil {
		return err
	}
	sendCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if err := s.client.TelemetryReportsSubmit(sendCtx, &report); err != nil {
		s.mod.log.DebugContext(ctx, "telemetry report not sent", "err", err)
		return nil
	}
	return s.mod.MarkReportSent(ctx, now)
}
