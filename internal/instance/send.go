package instance

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"

	"golang.org/x/mod/semver"

	"github.com/getsphericon/sphericon/config"
	telemetryapi "github.com/getsphericon/sphericon/gen/telemetry"
	"github.com/getsphericon/sphericon/internal/buildinfo"
)

const (
	// reportInterval is how often an Instance reports: once a day.
	reportInterval = 24 * time.Hour
	// reportJitter is the longest the next report is brought forward, so Instances that
	// started together do not all report in the same minute.
	reportJitter = 6 * time.Hour
	// reportTimeout bounds one send; there is no retry beyond the next cycle.
	reportTimeout = 5 * time.Second
)

// Sender sends the daily Telemetry report (ADR 0032). It sends nothing in a development
// environment or from a build that is not a release, and otherwise only while the effective
// setting is on, the Owner's notice is acknowledged and the last report is a day old.
type Sender struct {
	mod    *Module
	client *telemetryapi.Client
	dev    bool
	jitter func() time.Duration
}

// SenderOption tunes a Sender.
type SenderOption func(*Sender)

// WithJitter replaces the random head start of the next report (a test makes it exact).
func WithJitter(jitter func() time.Duration) SenderOption {
	return func(s *Sender) { s.jitter = jitter }
}

// NewSender builds the sender over the receiver address in cfg. A development environment
// (cfg.IsDev) never sends, so a test opts in by building its config as production.
func NewSender(mod *Module, cfg *config.Config, opts ...SenderOption) (*Sender, error) {
	client, err := telemetryapi.NewClient(cfg.TelemetryReceiverURL)
	if err != nil {
		return nil, err
	}
	s := &Sender{
		mod:    mod,
		client: client,
		dev:    cfg.IsDev,
		jitter: func() time.Duration { return rand.N(reportJitter) },
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// isReleaseBuild reports whether version is a release: a plain semantic version, not a
// development build or a prerelease.
func isReleaseBuild(version string) bool {
	v := buildinfo.Semver(version)
	return semver.IsValid(v) && semver.Prerelease(v) == "" && semver.Build(v) == ""
}

// Run sends the report when every gate lets it through. A failed send is logged at debug
// level and left for the next cycle; only a failure to read or store the state is an error.
func (s *Sender) Run(ctx context.Context) error {
	if s.dev || !isReleaseBuild(s.mod.host.AppVersion) {
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
	if last := row.LastReportSentAt; last != nil && now.Sub(*last) < reportInterval-s.jitter() {
		return nil
	}
	report, err := s.mod.Report(ctx)
	if err != nil {
		return err
	}
	sendCtx, cancel := context.WithTimeout(ctx, reportTimeout)
	defer cancel()
	if err := s.client.TelemetryReportsSubmit(sendCtx, &report); err != nil {
		slog.DebugContext(ctx, "telemetry report not sent", "err", err)
		return nil
	}
	return s.mod.ent.InstanceState.UpdateOneID(stateID).SetLastReportSentAt(now).Exec(ctx)
}
