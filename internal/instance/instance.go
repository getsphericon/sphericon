// Package instance owns the Instance's own state (ADR 0032): the one row that no Workspace
// owns (its random Instance id, the Telemetry report setting, the acknowledged notice) and
// the rule of who decides about it. It reads across Workspaces, so it holds the raw ent client.
package instance

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/instancestate"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/workspace"
	telemetryapi "github.com/getsphericon/sphericon/gen/telemetry"
)

// stateID is the id of the one Instance state row.
const stateID int64 = 1

// ErrTelemetryLocked is returned when the Owner tries to change a setting the environment decides.
var ErrTelemetryLocked = errors.New("the telemetry setting is decided by the environment")

// Environment is what the deployment says about the Telemetry report: SPHERICON_TELEMETRY
// when set to true or false, and DO_NOT_TRACK=1.
type Environment struct {
	Telemetry  *bool
	DoNotTrack bool
}

// EnvironmentOf reads the Telemetry report switches from the configuration.
func EnvironmentOf(cfg *config.Config) Environment {
	return Environment{Telemetry: cfg.Telemetry, DoNotTrack: cfg.DoNotTrack}
}

// Lock names what decides a setting the Owner cannot change.
type Lock string

const (
	// LockNone means the Owner decides.
	LockNone Lock = ""
	// LockEnvironment means SPHERICON_TELEMETRY decides.
	LockEnvironment Lock = "environment"
	// LockDoNotTrack means DO_NOT_TRACK=1 decides (always off).
	LockDoNotTrack Lock = "do_not_track"
)

// Setting is the Telemetry report setting as it applies: whether reports are sent and
// whether the environment (not the Owner) decides.
type Setting struct {
	Enabled  bool
	Locked   bool
	LockedBy Lock
}

// Effective applies ADR 0032 to the environment and the stored in-app setting (nil is
// unset, which is on): off if SPHERICON_TELEMETRY is false or DO_NOT_TRACK=1; on and
// locked if SPHERICON_TELEMETRY is true; otherwise the stored setting decides. DO_NOT_TRACK
// only ever switches off. The sender of the report reuses it.
func Effective(env Environment, stored *bool) Setting {
	switch {
	case env.Telemetry != nil && !*env.Telemetry:
		return Setting{Locked: true, LockedBy: LockEnvironment}
	case env.DoNotTrack:
		return Setting{Locked: true, LockedBy: LockDoNotTrack}
	case env.Telemetry != nil:
		return Setting{Enabled: true, Locked: true, LockedBy: LockEnvironment}
	}
	return Setting{Enabled: stored == nil || *stored}
}

// State is what an Owner sees about the Instance.
type State struct {
	Telemetry          Setting
	NoticeAcknowledged bool
}

// Module is the Instance state and the rules around it.
type Module struct {
	ent  *ent.Client
	env  Environment
	host Host
	now  func() time.Time
}

// New builds the module; a nil clock means time.Now. The host is what the Telemetry report
// says about the running binary.
func New(client *ent.Client, env Environment, host Host, now func() time.Time) *Module {
	if now == nil {
		now = time.Now
	}
	return &Module{ent: client, env: env, host: host, now: now}
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

// IsOwner reports whether the User is an Owner of the oldest existing Workspace: the next
// oldest when that one is deleted, every Owner when there are several, and a transfer
// follows the role. These Owners alone decide about the Instance.
func (m *Module) IsOwner(ctx context.Context, userID int64) (bool, error) {
	oldest, err := m.ent.Workspace.Query().Order(ent.Asc(workspace.FieldID)).FirstID(ctx)
	if ent.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return m.ent.Membership.Query().
		Where(membership.UserID(userID), membership.WorkspaceID(oldest), membership.RoleEQ(membership.RoleOwner)).
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

// State is the Instance state an Owner sees.
func (m *Module) State(ctx context.Context) (State, error) {
	row, err := m.Ensure(ctx)
	if err != nil {
		return State{}, err
	}
	return State{Telemetry: Effective(m.env, row.TelemetryEnabled), NoticeAcknowledged: row.NoticeAcknowledgedAt != nil}, nil
}

// SetTelemetry stores the Owner's choice; ErrTelemetryLocked when the environment decides.
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

// AcknowledgeNotice records that the one-time notice was seen; the first time stands.
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
