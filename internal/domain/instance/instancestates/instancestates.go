// Package instancestates owns the Instance's own state (ADR 0032): the one row that no Workspace
// owns (its random Instance id, the Telemetry report setting, the acknowledged notice, the
// Release check's result, the default Send cap of new Workspaces). A Staff member decides about it
// in the console (ADR 0063), and a change of the default Send cap is recorded in the Instance
// audit log (ADR 0070). The row belongs to no Workspace, and the Instance-wide reads behind the
// Telemetry report and the banner of a newer release span every Workspace, so it holds the raw ent
// client. The Release check (releasecheck.go) and the Telemetry report (telemetryreport.go) are
// the module's operations too (ADR 0112): each reads and writes the one row and asks one outside
// party, the release source and the receiver of the Telemetry contract, which the wiring hands in.
package instancestates

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/samber/lo"
	"golang.org/x/mod/semver"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/instancestate"
	"github.com/getsphericon/sphericon/ent/membership"
	telemetryapi "github.com/getsphericon/sphericon/gen/telemetry"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/instance/instanceauditentries"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/buildinfo"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// stateID is the id of the one Instance state row.
const stateID int64 = 1

// ErrTelemetryLocked is returned when a Staff member tries to change a setting the environment
// decides.
var ErrTelemetryLocked = validation.Conflict("instance: the telemetry setting is decided by the environment", i18n.ErrorsTelemetryLocked)

// Environment is what the deployment says about the Instance: its Mode's say over the Telemetry
// report (ADR 0060), DO_NOT_TRACK=1, whether the Mode makes the Release check and the version that
// runs.
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
	// Version is the version that runs, the one the Release check compares to.
	Version string
	// LicenseExpiredAt is when the license key expired; zero unless it has lapsed (ADR 0035).
	LicenseExpiredAt time.Time
}

// EnvironmentOf reads the Instance switches from the configuration, with the version that runs and
// when its license key lapsed (zero unless it has).
func EnvironmentOf(cfg *config.Config, version string, licenseExpiredAt time.Time) Environment {
	return Environment{Telemetry: cfg.Telemetry, DoNotTrack: cfg.DoNotTrack, ReleaseCheck: cfg.ReleaseCheck,
		Dev: cfg.IsDev, SchemaURL: cfg.TelemetrySchemaURL, Version: version, LicenseExpiredAt: licenseExpiredAt}
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
// only ever switches off. The send of the report (SendTelemetryReport) reuses it.
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
	// DefaultSendCap is the Send cap of new Workspaces; nil on an Instance that has no Send caps.
	DefaultSendCap *DefaultSendCap
}

// DefaultSendCap is the Send cap every new Workspace starts with (ADR 0067).
type DefaultSendCap struct {
	// PerDay is the messages per rolling 24 hours; nil when new Workspaces get no Send cap.
	PerDay *int
}

// DefaultSendCapChange is the outcome of setting or clearing the default Send cap: the default as
// it now stands, and whether it changed.
type DefaultSendCapChange struct {
	Changed        bool
	DefaultSendCap DefaultSendCap
}

// Module is the Instance state and the rules around it.
type Module struct {
	ent *ent.Client
	env Environment
	now func() time.Time
	// sendCaps is whether the Instance has Send caps (ADR 0067): only then does it have a default
	// Send cap.
	sendCaps providerlimit.SendCaps
	// releases is the Release check's source of the newest release; nil asks nobody.
	releases ReleaseSource
	// host is what the Telemetry report says about the running binary.
	host TelemetryHost
	// receiver is the client of the Telemetry report's receiver; nil sends nothing.
	receiver *telemetryapi.Client
	// log takes the Release check's and the Telemetry report's lines.
	log *slog.Logger
}

// New builds the module on the Instance clock now. sendCaps says whether the Instance has Send
// caps (ADR 0067), and so a default Send cap a Staff member sets. The Release check asks releases
// for the newest release (nil asks nobody). The Telemetry report says host about the running binary
// (read by the wiring) and is sent through receiver, the client of the Telemetry contract's
// receiver (built by the wiring from config.TelemetryReceiverURL; nil builds the report for the
// preview and never sends it). log takes the lines of both. now and log are required.
func New(client *ent.Client, env Environment, now func() time.Time, sendCaps providerlimit.SendCaps,
	releases ReleaseSource, host TelemetryHost, receiver *telemetryapi.Client, log *slog.Logger,
) *Module {
	if now == nil {
		panic("instancestates: the module needs the Instance clock")
	}
	if log == nil {
		panic("instancestates: the module needs a logger")
	}
	return &Module{ent: client, env: env, now: now, sendCaps: sendCaps, releases: releases, host: host, receiver: receiver, log: log}
}

// Ensure returns the Instance state row, creating it (with a fresh random Instance id)
// when this is the first start. Safe to call concurrently and repeatedly.
func (m *Module) Ensure(ctx context.Context) (*ent.InstanceState, error) {
	return ensureRow(ctx, m.ent)
}

func ensureRow(ctx context.Context, client *ent.Client) (*ent.InstanceState, error) {
	err := client.InstanceState.Create().SetID(stateID).OnConflictColumns(instancestate.FieldID).DoNothing().Exec(ctx)
	_, err = db.Inserted(err)
	if err != nil {
		return nil, err
	}
	return client.InstanceState.Get(ctx, stateID)
}

// Environment is what the deployment says about the Instance, as the module was built with it.
func (m *Module) Environment() Environment { return m.env }

// holdsReleaseRead is whether the User userID holds the Permission release:read (an owner, ADR
// 0061) in a live Workspace: who is told of a newer release. It reads the Memberships of every
// Workspace.
func (m *Module) holdsReleaseRead(ctx context.Context, userID int64) (bool, error) {
	return m.ent.Membership.Query().
		Where(membership.UserID(userID), memberships.HoldersOf(memberships.PermReleaseRead), membership.HasWorkspaceWith(workspaces.Live())).
		Exist(ctx)
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
	s := State{
		Telemetry:          Effective(m.env, row.TelemetryEnabled),
		NoticeAcknowledged: row.NoticeAcknowledgedAt != nil,
		LatestRelease:      lo.EmptyableToPtr(m.newer(row.LatestRelease)),
		LicenseExpiredAt:   m.env.LicenseExpiredAt,
		TelemetrySchemaURL: m.env.SchemaURL,
	}
	if m.sendCaps {
		s.DefaultSendCap = &DefaultSendCap{PerDay: row.DefaultSendCap}
	}
	return s, nil
}

// SetDefaultSendCap sets the Send cap of new Workspaces to perDay messages per rolling 24 hours, or
// clears it with nil, as the Staff member by (ADR 0067). The change is recorded in the Instance
// audit log with the old and the new value, in the same transaction; setting the same value or
// clearing none is a no-op and records nothing. Existing Workspaces keep their Send caps.
// providerlimit.ErrNoSendCaps on an Instance without Send caps; a Send cap outside the contract's
// bounds (bounds.MaxPerDay) is refused.
func (m *Module) SetDefaultSendCap(ctx context.Context, by *ent.StaffMember, perDay *int) (DefaultSendCapChange, error) {
	if err := m.sendCaps.Check(); err != nil {
		return DefaultSendCapChange{}, err
	}
	if perDay != nil {
		if err := validation.Range("perDay", *perDay, bounds.MaxPerDayMin, bounds.MaxPerDayMax); err != nil {
			return DefaultSendCapChange{}, err
		}
	}
	if _, err := m.Ensure(ctx); err != nil {
		return DefaultSendCapChange{}, err
	}
	tx, err := m.ent.Tx(ctx)
	if err != nil {
		return DefaultSendCapChange{}, fmt.Errorf("set the default send cap: %w", err)
	}
	changed, err := setDefaultSendCap(ctx, tx.Client(), by, perDay)
	if err != nil {
		return DefaultSendCapChange{}, errors.Join(fmt.Errorf("set the default send cap: %w", err), tx.Rollback())
	}
	if err := tx.Commit(); err != nil {
		return DefaultSendCapChange{}, fmt.Errorf("set the default send cap: %w", err)
	}
	return DefaultSendCapChange{Changed: changed, DefaultSendCap: DefaultSendCap{PerDay: perDay}}, nil
}

// setDefaultSendCap changes the stored default on tx and records the change, unless it is the
// same; it reports whether it changed.
func setDefaultSendCap(ctx context.Context, tx *ent.Client, by *ent.StaffMember, perDay *int) (bool, error) {
	row, err := tx.InstanceState.Query().Where(instancestate.ID(stateID)).ForUpdate().Only(ctx)
	if err != nil {
		return false, err
	}
	change, differs := events.ChangeOf(row.DefaultSendCap, perDay)
	if !differs {
		return false, nil
	}
	upd := tx.InstanceState.UpdateOneID(stateID)
	if perDay == nil {
		upd.ClearDefaultSendCap()
	} else {
		upd.SetDefaultSendCap(*perDay)
	}
	if err := upd.Exec(ctx); err != nil {
		return false, err
	}
	diff := map[string]any{instancestate.FieldDefaultSendCap: change}
	return true, instanceauditentries.Record(ctx, tx, by.ID, instanceauditentries.ActionDefaultSendCapChange, diff)
}

// StartingSendCapIn is the Send cap a Workspace created now starts with (ADR 0067), read on tx, the
// transaction that creates it: on an Instance with Send caps the stored default, nil when none is
// set or the Instance state row does not exist yet; on one without them, none, whatever default is
// stored. The Workspace creation and the Sign-up, which creates a User's first Workspace, both ask
// it.
func (m *Module) StartingSendCapIn(ctx context.Context, tx *ent.Client) (*int, error) {
	if !m.sendCaps {
		return nil, nil
	}
	return defaultSendCapIn(ctx, tx)
}

// defaultSendCapIn is the stored default Send cap read on tx, nil when none is set or the Instance
// state row does not exist yet.
func defaultSendCapIn(ctx context.Context, tx *ent.Client) (*int, error) {
	row, err := tx.InstanceState.Query().Where(instancestate.ID(stateID)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the default send cap: %w", err)
	}
	return row.DefaultSendCap, nil
}

// AuditEntries is a page of the Instance audit log, newest first, from the cursor a previous page
// handed out (see instanceauditentries.Entries).
func (m *Module) AuditEntries(ctx context.Context, cursor string, limit int) (instanceauditentries.Page, error) {
	return instanceauditentries.Entries(ctx, m.ent, cursor, limit)
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
	if _, err := m.Ensure(ctx); err != nil {
		return State{}, err
	}
	err := lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
		n, err := m.ent.InstanceState.Update().
			Where(instancestate.ID(stateID), instancestate.NoticeAcknowledgedAtIsNil()).
			SetNoticeAcknowledgedAt(m.now()).
			Save(ctx)
		return n > 0, err
	})
	// Acknowledged before: the first moment stands.
	if err != nil && !errors.Is(err, lifecycle.ErrInvalidTransition) {
		return State{}, err
	}
	return m.State(ctx)
}

// markReportSent stores when the last Telemetry report went out.
func (m *Module) markReportSent(ctx context.Context, at time.Time) error {
	if _, err := m.Ensure(ctx); err != nil {
		return err
	}
	return m.ent.InstanceState.UpdateOneID(stateID).SetLastReportSentAt(at).Exec(ctx)
}

// ChecksReleases is whether the Release check runs at all: it is switched on, this is not a
// development or test environment and the running build is a release.
func (m *Module) ChecksReleases() bool {
	return m.env.ReleaseCheck && !m.env.Dev && buildinfo.IsRelease(m.env.Version)
}

// isNewer is whether latest is a stable release strictly newer than the running version.
func (m *Module) isNewer(latest string) bool {
	return buildinfo.IsRelease(latest) && semver.Compare(buildinfo.Semver(latest), buildinfo.Semver(m.env.Version)) > 0
}

// newer is the stored release when it is still newer than the running version (an Instance
// upgraded since the check stops announcing it) and the check is on; empty otherwise.
func (m *Module) newer(stored *string) string {
	if stored == nil || !m.ChecksReleases() || !m.isNewer(*stored) {
		return ""
	}
	return *stored
}

// LatestRelease is the cached result of the Release check: a stable release newer than the
// running version, or empty. It asks nobody.
func (m *Module) LatestRelease(ctx context.Context) (string, error) {
	row, err := m.Ensure(ctx)
	if err != nil {
		return "", err
	}
	return m.newer(row.LatestRelease), nil
}

// recordRelease stores the newest release the Release check found, with the check time, only
// when the check runs and latest is a stable release strictly newer than the running version, and
// reports whether it stored one that differs from the one stored before.
func (m *Module) recordRelease(ctx context.Context, latest string) (bool, error) {
	if !m.ChecksReleases() || !m.isNewer(latest) {
		return false, nil
	}
	row, err := m.Ensure(ctx)
	if err != nil {
		return false, err
	}
	err = m.ent.InstanceState.UpdateOneID(stateID).SetLatestRelease(latest).SetLatestReleaseCheckedAt(m.now()).Exec(ctx)
	return row.LatestRelease == nil || *row.LatestRelease != latest, err
}
