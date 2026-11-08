// Package workspaces owns the Workspace (ADR 0043, ADR 0102): the tenant every other row belongs
// to, its settings, whether it is live, and the deletion window with the erase date. It sits at
// the bottom of identity, so every module can read a Workspace's settings and its window.
//
// The Workspace is the tenant root, not a tenant-owned entity, so the scoped client has no
// wrapper for it: the module writes the row with the raw client, always the row of the
// Workspace a scope names, and resolves a Workspace id for the command line.
package workspaces

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/currency"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/reportday"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Module reads and writes the Workspace row over the raw client.
type Module struct {
	client *ent.Client
	bus    *events.Bus
}

// New builds the module over the ent client; the bus opens the transactions that write a
// setting with its Audit entry.
func New(client *ent.Client, bus *events.Bus) *Module {
	return &Module{client: client, bus: bus}
}

// DeletionWindow is how long a deleted Workspace stays restorable before its data is
// erased (ADR 0043). A constant, not configuration.
const DeletionWindow = 30 * 24 * time.Hour

// EraseDate is when the data of a Workspace deleted at discardedAt is erased.
func EraseDate(discardedAt time.Time) time.Time { return discardedAt.Add(DeletionWindow) }

// ErrNotFound is the refusal of a Workspace that does not exist, is not live, or that
// the User is not a member of. The cases are one refusal, so the answer does not tell a caller
// which Workspace ids exist. Every surface answers it alike (review rule 15).
var ErrNotFound = errors.New("workspaces: workspace not found")

// BootstrapScope is the scoped client of the oldest Workspace, for the bootstrap
// token (no caller token exists yet to say which Workspace is meant).
func (m *Module) BootstrapScope(ctx context.Context) (*ent.Scoped, error) {
	id, err := m.client.Workspace.Query().Where(Live()).Order(ent.Asc(workspace.FieldID)).FirstID(ctx)
	if err != nil {
		return nil, err
	}
	return events.Ingest(m.client.Scoped(id)), nil
}

// LinkScope is the scoped client of the Workspace a signed unsubscribe or confirmation link
// names: the Workspace comes from the token (no membership or API token stands behind it), so its writes
// are the ingest actor's, never audited.
func (m *Module) LinkScope(workspaceID int64) *ent.Scoped {
	return events.Ingest(m.client.Scoped(workspaceID))
}

// Update renames the scoped Workspace and, when postalAddress is non-nil,
// sets (or clears, with "") its postal address. A change is recorded as a
// `workspace.update` Audit entry in the same transaction (ADR 0022): the Workspace is
// the tenant root and has no scoped wrapper, so this is an explicit path.
// The name is trimmed; an empty one is ErrNameEmpty.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, actor events.Actor, name string, postalAddress *string) (*ent.Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNameEmpty
	}
	var updated *ent.Workspace
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		before, err := tx.Workspace.Get(ctx, s.WorkspaceID())
		if err != nil {
			return err
		}
		upd := tx.Workspace.UpdateOneID(s.WorkspaceID()).SetName(name)
		if postalAddress != nil {
			upd = upd.SetPostalAddress(*postalAddress)
		}
		updated, err = upd.Save(ctx)
		if err != nil {
			return err
		}
		diff := map[string]any{}
		if before.Name != updated.Name {
			diff["name"] = events.Change(before.Name, updated.Name)
		}
		if before.PostalAddress != updated.PostalAddress {
			diff["postal_address"] = events.Change(before.PostalAddress, updated.PostalAddress)
		}
		if len(diff) == 0 {
			return nil
		}
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: s.WorkspaceID(),
			Actor:       actor,
			Action:      events.ActionWorkspaceUpdate,
			TargetType:  workspace.Label,
			TargetID:    strconv.FormatInt(updated.ID, 10),
			TargetName:  updated.Name,
			Diff:        diff,
		})
	})
	return updated, err
}

// SetAuditRetention sets (or, with nil, clears) the number of days the Audit log is
// kept (the EE advanced-retention window, ADR 0014) and records the change as a
// `workspace.update` Audit entry in the same transaction. The caller has checked the
// license and the role.
func (m *Module) SetAuditRetention(ctx context.Context, s *ent.Scoped, actor events.Actor, days *int) error {
	return m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		before, err := tx.Workspace.Get(ctx, s.WorkspaceID())
		if err != nil {
			return err
		}
		upd := tx.Workspace.UpdateOneID(s.WorkspaceID())
		if days == nil {
			upd = upd.ClearRetentionDays()
		} else {
			upd = upd.SetRetentionDays(*days)
		}
		if _, err := upd.Save(ctx); err != nil {
			return err
		}
		if ptrEqual(before.RetentionDays, days) {
			return nil
		}
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: s.WorkspaceID(),
			Actor:       actor,
			Action:      events.ActionWorkspaceUpdate,
			TargetType:  workspace.Label,
			TargetID:    strconv.FormatInt(before.ID, 10),
			TargetName:  before.Name,
			Diff:        map[string]any{"retention_days": events.Change(before.RetentionDays, days)},
		})
	})
}

// SetTimezone sets the Workspace time zone (ADR 0057) and records the change as a
// `workspace.update` Audit entry in the same transaction. A name the zone database does not
// know is refused with ErrUnknownZone. The caller has checked the role.
func (m *Module) SetTimezone(ctx context.Context, s *ent.Scoped, actor events.Actor, name string) (*ent.Workspace, error) {
	if err := CheckZone(name); err != nil {
		return nil, err
	}
	var updated *ent.Workspace
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		before, err := tx.Workspace.Get(ctx, s.WorkspaceID())
		if err != nil {
			return err
		}
		updated, err = tx.Workspace.UpdateOneID(s.WorkspaceID()).SetTimezone(name).Save(ctx)
		if err != nil {
			return err
		}
		from, to := reportday.Name(before.Timezone), reportday.Name(updated.Timezone)
		if from == to {
			return nil
		}
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: s.WorkspaceID(),
			Actor:       actor,
			Action:      events.ActionWorkspaceUpdate,
			TargetType:  workspace.Label,
			TargetID:    strconv.FormatInt(before.ID, 10),
			TargetName:  before.Name,
			Diff:        map[string]any{workspace.FieldTimezone: events.Change(from, to)},
		})
	})
	return updated, err
}

// ErrUnknownCurrency refuses a Reporting currency code that is not in ISO 4217.
var ErrUnknownCurrency = validation.RefuseField("reportingCurrency", "workspaces: unknown currency code", i18n.ErrorsCurrencyUnknown)

// ErrUnknownZone refuses a Workspace or a User time zone the zone database does not know. It
// wraps reportday.ErrUnknownZone.
var ErrUnknownZone = validation.RefuseField("timezone", "workspaces: unknown time zone", i18n.ErrorsTimezoneUnknown)

// CheckZone refuses with ErrUnknownZone an IANA name the zone database does not know.
func CheckZone(name string) error {
	if _, err := reportday.Load(name); err != nil {
		if errors.Is(err, reportday.ErrUnknownZone) {
			return fmt.Errorf("%w: %w", ErrUnknownZone, err)
		}
		return err
	}
	return nil
}

// SetRevenueSettings changes the Reporting currency and the Attribution window (a nil field stays
// as it is) and records each real change in one `workspace.update` Audit entry in the same
// transaction (ADR 0064). A code outside ISO 4217 is refused with ErrUnknownCurrency; the bounds of
// the window are the contract's. The caller has checked the role.
func (m *Module) SetRevenueSettings(ctx context.Context, s *ent.Scoped, actor events.Actor, code *string, windowDays *int) (*ent.Workspace, error) {
	if code != nil {
		unit, err := currency.ParseISO(*code)
		if err != nil {
			return nil, ErrUnknownCurrency
		}
		canonical := unit.String()
		code = &canonical
	}
	var updated *ent.Workspace
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		before, err := tx.Workspace.Get(ctx, s.WorkspaceID())
		if err != nil {
			return err
		}
		diff := map[string]any{}
		upd := tx.Workspace.UpdateOneID(s.WorkspaceID())
		if code != nil && (before.ReportingCurrency == nil || *before.ReportingCurrency != *code) {
			upd = upd.SetReportingCurrency(*code)
			diff[workspace.FieldReportingCurrency] = events.Change(before.ReportingCurrency, *code)
		}
		if windowDays != nil && before.AttributionWindowDays != *windowDays {
			upd = upd.SetAttributionWindowDays(*windowDays)
			diff[workspace.FieldAttributionWindowDays] = events.Change(before.AttributionWindowDays, *windowDays)
		}
		if len(diff) == 0 {
			updated = before
			return nil
		}
		updated, err = upd.Save(ctx)
		if err != nil {
			return err
		}
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: s.WorkspaceID(),
			Actor:       actor,
			Action:      events.ActionWorkspaceUpdate,
			TargetType:  workspace.Label,
			TargetID:    strconv.FormatInt(before.ID, 10),
			TargetName:  before.Name,
			Diff:        diff,
		})
	})
	return updated, err
}

// AdoptReportingCurrency makes code the Reporting currency of the scoped Workspace when it has
// none yet (ADR 0064): the currency of the first Order Completed the Workspace sends. It changes
// nothing once one is set, by an Owner or Admin or by an earlier Order. The change is a
// `workspace.update` Audit entry by the system, in a transaction of its own (the orders
// subscriber calls it before it opens the Event's), and the conditional write makes the first of
// two Orders read at once win. code is a currency the order Event check has accepted.
func (m *Module) AdoptReportingCurrency(ctx context.Context, s *ent.Scoped, code string) error {
	return m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		n, err := tx.Workspace.Update().
			Where(workspace.ID(s.WorkspaceID()), workspace.ReportingCurrencyIsNil()).
			SetReportingCurrency(code).
			Save(ctx)
		if err != nil || n == 0 {
			return err
		}
		ws, err := tx.Workspace.Get(ctx, s.WorkspaceID())
		if err != nil {
			return err
		}
		diff := map[string]any{workspace.FieldReportingCurrency: events.Became(code)}
		return events.RecordAudit(ctx, pub,
			events.WorkspaceAudit(ws, events.Actor{Kind: events.ActorSystem}, events.ActionWorkspaceUpdate, diff))
	})
}

// MarkSetupComplete stamps the scoped Workspace's Setup checklist complete at now, once: a
// Workspace already stamped keeps its first moment. It returns the moment the Workspace holds.
// Nothing a member chose changes, so no Audit entry is written.
func (m *Module) MarkSetupComplete(ctx context.Context, s *ent.Scoped, now time.Time) (time.Time, error) {
	err := m.client.Workspace.Update().
		Where(workspace.ID(s.WorkspaceID()), workspace.SetupCompletedAtIsNil()).
		SetSetupCompletedAt(now).
		Exec(ctx)
	if err != nil {
		return time.Time{}, err
	}
	ws, err := m.client.Workspace.Get(ctx, s.WorkspaceID())
	if err != nil {
		return time.Time{}, err
	}
	return *ws.SetupCompletedAt, nil
}

// SetSecondFactorRequirement switches the scoped Workspace's Two-factor requirement
// (ADR 0020) on, starting at now, or off. Switching it on while it is on keeps the
// original start, so no member's grace restarts. A change is recorded as a
// `workspace.update` Audit entry in the same transaction, and switching it on publishes
// events.SecondFactorRequired there too, which tells the members without a Second factor
// (ADR 0105). The caller has checked the role.
func (m *Module) SetSecondFactorRequirement(ctx context.Context, s *ent.Scoped, actor events.Actor, required bool, now time.Time) (updated *ent.Workspace, err error) {
	err = m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		before, err := tx.Workspace.Get(ctx, s.WorkspaceID())
		if err != nil {
			return err
		}
		updated = before
		if (before.SecondFactorRequiredAt != nil) == required {
			return nil
		}
		upd := tx.Workspace.UpdateOneID(s.WorkspaceID())
		if required {
			upd = upd.SetSecondFactorRequiredAt(now)
		} else {
			upd = upd.ClearSecondFactorRequiredAt()
		}
		if updated, err = upd.Save(ctx); err != nil {
			return err
		}
		if err := events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: s.WorkspaceID(),
			Actor:       actor,
			Action:      events.ActionWorkspaceUpdate,
			TargetType:  workspace.Label,
			TargetID:    strconv.FormatInt(before.ID, 10),
			TargetName:  before.Name,
			Diff:        map[string]any{"second_factor_required_at": events.Change(before.SecondFactorRequiredAt, updated.SecondFactorRequiredAt)},
		}); err != nil || !required {
			return err
		}
		return pub.Publish(ctx, &events.SecondFactorRequired{WorkspaceID: s.WorkspaceID()})
	})
	return updated, err
}

func ptrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// ErrNameEmpty refuses a Workspace name that is empty once trimmed.
var ErrNameEmpty = validation.RefuseField("name", "workspaces: workspace name is empty", i18n.ErrorsNameEmpty)

// Get is the scoped Workspace's own row.
func (m *Module) Get(ctx context.Context, s *ent.Scoped) (*ent.Workspace, error) {
	return s.Workspace(ctx)
}

// IDByPublicIDIn resolves a Workspace id (ADR 0043) to the internal key, in the caller's
// transaction: an operator addresses a Workspace by its id (a suspension from the command line).
func IDByPublicIDIn(ctx context.Context, client *ent.Client, publicID string) (int64, error) {
	id, err := client.Workspace.Query().Where(workspace.PublicID(publicID)).OnlyID(ctx)
	if err != nil {
		return 0, fmt.Errorf("workspace %q: %w", publicID, err)
	}
	return id, nil
}

// Live is a Workspace that is not deleted (ADR 0043): every lookup that must not reach a
// deleted Workspace (its Members' scope, the lists, the Instance-wide readers) uses it, so
// "live" is stated once.
func Live() predicate.Workspace { return workspace.DiscardedAtIsNil() }

// ErasedIn reports whether the scope's Workspace is erased (ADR 0043): its rows are gone and its
// tombstone is all that is left, so a write on its behalf that arrives late (a signed link clicked
// long after) has nothing to record into. A Workspace in its deletion window is not erased: a
// restore brings it back with what was recorded meanwhile.
func ErasedIn(ctx context.Context, s *ent.Scoped) (bool, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return false, err
	}
	return ws.ErasedAt != nil, nil
}

// Deleted is a Workspace in its deletion window or erased (ADR 0043): the complement of Live.
func Deleted() predicate.Workspace { return workspace.DiscardedAtNotNil() }

// SetDomainJoin switches the scoped Workspace's domain join on or off (ADR 0051). Switching
// it on adds nobody: only an address proven afterwards joins. A change is recorded as a
// `workspace.update` Audit entry in the same transaction; the Workspace is not an Audited
// entity, so this is an explicit path like SetSecondFactorRequirement. The caller has
// checked the role.
func (m *Module) SetDomainJoin(ctx context.Context, s *ent.Scoped, actor events.Actor, enabled bool) (*ent.Workspace, error) {
	var updated *ent.Workspace
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		before, err := tx.Workspace.Get(ctx, s.WorkspaceID())
		if err != nil {
			return err
		}
		updated = before
		if before.DomainJoinEnabled == enabled {
			return nil
		}
		if updated, err = tx.Workspace.UpdateOneID(before.ID).SetDomainJoinEnabled(enabled).Save(ctx); err != nil {
			return err
		}
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: before.ID,
			Actor:       actor,
			Action:      events.ActionWorkspaceUpdate,
			TargetType:  workspace.Label,
			TargetID:    strconv.FormatInt(before.ID, 10),
			TargetName:  before.Name,
			Diff:        map[string]any{"domain_join_enabled": events.Change(before.DomainJoinEnabled, enabled)},
		})
	})
	return updated, err
}

// DomainJoinEnabled reports whether the scoped Workspace admits Users by Join domain.
func (m *Module) DomainJoinEnabled(ctx context.Context, s *ent.Scoped) (bool, error) {
	w, err := m.client.Workspace.Get(ctx, s.WorkspaceID())
	if err != nil {
		return false, err
	}
	return w.DomainJoinEnabled, nil
}
