// Package instance owns the Instance's own state (ADR 0032): the one row that no Workspace
// owns (its random Instance id, the Telemetry report setting, the acknowledged notice, the
// Release check's result). A Staff member decides about it in the console (ADR 0063); the
// banner of a newer release reads the Memberships of every Workspace, so it holds the raw ent
// client.
package instance

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/instancestate"
	"github.com/getsphericon/sphericon/ent/membership"
	telemetryapi "github.com/getsphericon/sphericon/gen/telemetry"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/validation"
)

// stateID is the id of the one Instance state row.
const stateID int64 = 1

// ErrTelemetryLocked is returned when a Staff member tries to change a setting the environment
// decides.
var ErrTelemetryLocked = validation.Conflict("instance: the telemetry setting is decided by the environment", i18n.ErrorsTelemetryLocked)

// Environment is what the deployment says about the Instance: its Mode's say over the Telemetry
// report (ADR 0060), DO_NOT_TRACK=1, whether the Mode makes the Release check and the version
// that runs.
type Environment struct {
	Telemetry  *bool
	DoNotTrack bool
	// ReleaseCheck is whether the Mode makes the Release check; it never depends on the two above.
	ReleaseCheck bool
	// Dev is a development or test environment: neither the Telemetry report nor the
	// Release check ever runs there.
	Dev bool
	// SchemaURL is the page that documents the Telemetry report's schema.
	SchemaURL string
}

// EnvironmentOf reads the Instance switches from the configuration.
func EnvironmentOf(cfg *config.Config) Environment {
	return Environment{Telemetry: cfg.Telemetry, DoNotTrack: cfg.DoNotTrack, ReleaseCheck: cfg.ReleaseCheck,
		Dev: cfg.IsDev, SchemaURL: cfg.TelemetrySchemaURL}
}

// Lock names what decides a setting the Staff member cannot change.
type Lock string

const (
	// LockNone means the Staff member decides.
	LockNone Lock = ""
	// LockEnvironment means the deployment (the saas Mode) decides.
	LockEnvironment Lock = "environment"
	// LockDoNotTrack means DO_NOT_TRACK=1 decides (always off).
	LockDoNotTrack Lock = "do_not_track"
)

// Setting is the Telemetry report setting as it applies: whether reports are sent and
// whether the environment (not the Staff member) decides.
type Setting struct {
	Enabled  bool
	Locked   bool
	LockedBy Lock
}

// Effective applies ADR 0032 to the environment and the stored setting (on until a Staff
// member switches it off in the console): off if the Mode says so (saas) or DO_NOT_TRACK=1; on and
// locked if the Mode says so; otherwise the stored setting decides. DO_NOT_TRACK
// only ever switches off. The sender of the report reuses it.
func Effective(env Environment, stored bool) Setting {
	switch {
	case env.Telemetry != nil && !*env.Telemetry:
		return Setting{Locked: true, LockedBy: LockEnvironment}
	case env.DoNotTrack:
		return Setting{Locked: true, LockedBy: LockDoNotTrack}
	case env.Telemetry != nil:
		return Setting{Enabled: true, Locked: true, LockedBy: LockEnvironment}
	}
	return Setting{Enabled: stored}
}

// State is what a Staff member sees about the Instance.
type State struct {
	Telemetry          Setting
	NoticeAcknowledged bool
	// LatestRelease is the bare version of a newer stable release, nil when there is none.
	LatestRelease *string
	// LicenseExpiredAt is when the license key expired; zero unless it has lapsed (ADR 0035).
	LicenseExpiredAt time.Time
	// TelemetrySchemaURL is the page that documents the Telemetry report's schema.
	TelemetrySchemaURL string
}

// Release is what a Workspace's owner sees of the Instance: the Release check's result.
type Release struct {
	// LatestRelease is the bare version of a newer stable release, nil when there is none.
	LatestRelease *string
}

// Module is the Instance state and the rules around it.
type Module struct {
	ent    *ent.Client
	env    Environment
	host   Host
	now    func() time.Time
	source ReleaseSource
	log    *slog.Logger
}

// Option configures the module.
type Option func(*Module)

// WithReleaseSource sets where the Release check asks for the newest release. Without one
// the check does nothing.
func WithReleaseSource(s ReleaseSource) Option { return func(m *Module) { m.source = s } }

// WithLogger sets the logger of the Release check's lines; the default is slog's.
func WithLogger(l *slog.Logger) Option { return func(m *Module) { m.log = l } }

// New builds the module; a nil clock means time.Now. The host is what the Telemetry report
// says about the running binary; its AppVersion is the version the Release check compares to.
func New(client *ent.Client, env Environment, host Host, now func() time.Time, opts ...Option) *Module {
	if now == nil {
		now = time.Now
	}
	m := &Module{ent: client, env: env, host: host, now: now, log: slog.Default()}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Ensure returns the Instance state row, creating it (with a fresh random Instance id)
// when this is the first start. Safe to call concurrently and repeatedly.
func (m *Module) Ensure(ctx context.Context) (*ent.InstanceState, error) {
	return ensureRow(ctx, m.ent)
}

func ensureRow(ctx context.Context, client *ent.Client) (*ent.InstanceState, error) {
	err := client.InstanceState.Create().SetID(stateID).OnConflictColumns(instancestate.FieldID).DoNothing().Exec(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return client.InstanceState.Get(ctx, stateID)
}

// Report is the Telemetry report as the Instance would send it now, whether or not sending
// is on: the same builder the daily sender uses.
func (m *Module) Report(ctx context.Context) (telemetryapi.TelemetryReport, error) {
	return BuildReport(ctx, m.ent, m.host, m.now())
}

// ReleaseFor is the banner of a newer release for the User userID: a User who holds the
// Permission release:read (an owner, ADR 0061) in a live Workspace is told, everyone else gets
// that Permission's refusal (403 permission_required). The Instance's settings are not a
// Workspace's business: they are the Staff console's (ADR 0063).
func (m *Module) ReleaseFor(ctx context.Context, userID int64) (Release, error) {
	held, err := m.ent.Membership.Query().
		Where(membership.UserID(userID), accounts.HoldersOf(accounts.PermReleaseRead), membership.HasWorkspaceWith(accounts.Live())).
		Exist(ctx)
	if err != nil {
		return Release{}, err
	}
	if !held {
		return Release{}, accounts.Refusal(accounts.PermReleaseRead)
	}
	latest, err := m.LatestRelease(ctx)
	if err != nil {
		return Release{}, err
	}
	return Release{LatestRelease: present(latest)}, nil
}

// present is the newer release as the resources carry it: nil when there is none.
func present(latest string) *string {
	if latest == "" {
		return nil
	}
	return &latest
}

// Setting is the effective Telemetry report setting right now.
func (m *Module) Setting(ctx context.Context) (Setting, error) {
	row, err := m.Ensure(ctx)
	if err != nil {
		return Setting{}, err
	}
	return Effective(m.env, row.TelemetryEnabled), nil
}

// State is the Instance state a Staff member sees.
func (m *Module) State(ctx context.Context) (State, error) {
	row, err := m.Ensure(ctx)
	if err != nil {
		return State{}, err
	}
	return State{
		Telemetry:          Effective(m.env, row.TelemetryEnabled),
		NoticeAcknowledged: row.NoticeAcknowledgedAt != nil,
		LatestRelease:      present(m.newer(row.LatestRelease)),
		LicenseExpiredAt:   m.host.LicenseExpiredAt,
		TelemetrySchemaURL: m.env.SchemaURL,
	}, nil
}

// SetTelemetry stores the Staff member's choice; ErrTelemetryLocked when the environment decides.
func (m *Module) SetTelemetry(ctx context.Context, enabled bool) (State, error) {
	row, err := m.Ensure(ctx)
	if err != nil {
		return State{}, err
	}
	if Effective(m.env, row.TelemetryEnabled).Locked {
		return State{}, ErrTelemetryLocked
	}
	if err := m.ent.InstanceState.UpdateOneID(stateID).SetTelemetryEnabled(enabled).Exec(ctx); err != nil {
		return State{}, err
	}
	return m.State(ctx)
}

// AcknowledgeNotice records that a Staff member saw the one-time notice; the first time stands.
// No report is sent before it.
func (m *Module) AcknowledgeNotice(ctx context.Context) (State, error) {
	row, err := m.Ensure(ctx)
	if err != nil {
		return State{}, err
	}
	if row.NoticeAcknowledgedAt == nil {
		if err := m.ent.InstanceState.UpdateOneID(stateID).SetNoticeAcknowledgedAt(m.now()).Exec(ctx); err != nil {
			return State{}, err
		}
	}
	return m.State(ctx)
}

// MarkReportSent stores when the last Telemetry report went out.
func (m *Module) MarkReportSent(ctx context.Context, at time.Time) error {
	if _, err := m.Ensure(ctx); err != nil {
		return err
	}
	return m.ent.InstanceState.UpdateOneID(stateID).SetLastReportSentAt(at).Exec(ctx)
}

// recordRelease stores the newest release the check found with the check time, and reports
// whether it differs from the one stored before.
func (m *Module) recordRelease(ctx context.Context, latest string) (bool, error) {
	row, err := m.Ensure(ctx)
	if err != nil {
		return false, err
	}
	err = m.ent.InstanceState.UpdateOneID(stateID).SetLatestRelease(latest).SetLatestReleaseCheckedAt(m.now()).Exec(ctx)
	return row.LatestRelease == nil || *row.LatestRelease != latest, err
}
