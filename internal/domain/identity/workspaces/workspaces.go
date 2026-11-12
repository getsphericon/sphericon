// Package workspaces owns the Workspace (ADR 0043, ADR 0102): the tenant every other row belongs
// to, its settings, whether it is live, its deletion (checked by its typed name, ADR 0110) and the
// deletion window with the erase date. It sits at the bottom of identity, so every module can read
// a Workspace's settings and its window.
//
// The Workspace is the tenant root, not a tenant-owned entity, so the scoped client has no
// wrapper for it: the module writes the row with the raw client, always the row of the
// Workspace a scope names, and resolves a Workspace id for the command line.
package workspaces

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/publicid"

	"golang.org/x/text/currency"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/reportday"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Module reads and writes the Workspace row over the raw client.
type Module struct {
	client *ent.Client
	bus    *events.Bus
	now    func() time.Time
}

// New builds the module over the ent client; the bus opens the transactions that write a
// setting with its Audit entry, and now is the Instance clock a deletion is dated by.
func New(client *ent.Client, bus *events.Bus, now func() time.Time) *Module {
	return &Module{client: client, bus: bus, now: now}
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

// OldestLiveID is the key of the oldest live Workspace, the one the bootstrap token mints the
// Instance's first token into (apitokens.Module.Bootstrap): no caller token exists yet to say which
// Workspace is meant.
func (m *Module) OldestLiveID(ctx context.Context) (int64, error) {
	return m.client.Workspace.Query().Where(Live()).Order(ent.Asc(workspace.FieldID)).FirstID(ctx)
}

// LinkScope is the scoped client of the Workspace a signed unsubscribe or confirmation link
// names: the Workspace comes from the token (no membership or API token stands behind it), so its writes
// are the ingest actor's, never audited.
func (m *Module) LinkScope(workspaceID int64) *ent.Scoped {
	return events.Ingest(m.client.Scoped(workspaceID))
}

// Update renames the scoped Workspace and, when postalAddress is non-nil,
// sets (or clears, with "") its postal address, trimmed so whitespace alone clears it. A change is recorded as a
// `workspace.update` Audit entry in the same transaction (ADR 0022): the Workspace is
// the tenant root and has no scoped wrapper, so this is an explicit path.
// The name is held to the name rule (validation.Name). revision is the Workspace's the edit was made
// on (ADR 0119): ErrRevisionChanged when it has moved past it, and an edit that changes nothing
// writes nothing.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, actor events.Actor, revision int64, name string, postalAddress *string) (*ent.Workspace, error) {
	name, err := validation.Name(name)
	if err != nil {
		return nil, err
	}
	var updated *ent.Workspace
	err = m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		before, err := editedAt(ctx, tx, s.WorkspaceID(), revision)
		if err != nil {
			return err
		}
		address := before.PostalAddress
		if postalAddress != nil {
			address = strings.TrimSpace(*postalAddress)
		}
		updated = before
		if name == before.Name && address == before.PostalAddress {
			return nil
		}
		updated, err = saveAt(ctx, tx.Workspace.UpdateOneID(s.WorkspaceID()).SetName(name).SetPostalAddress(address), revision)
		if err != nil {
			return err
		}
		diff := map[string]any{}
		if before.Name != updated.Name {
			diff[workspace.FieldName] = events.Change(before.Name, updated.Name)
		}
		if before.PostalAddress != updated.PostalAddress {
			diff[workspace.FieldPostalAddress] = events.Change(before.PostalAddress, updated.PostalAddress)
		}
		if len(diff) == 0 {
			return nil
		}
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: s.WorkspaceID(),
			Actor:       actor,
			Action:      events.ActionWorkspaceUpdate,
			TargetType:  workspace.Label,
			TargetID:    publicid.Encode(updated.ID),
			TargetName:  updated.Name,
			Diff:        diff,
		})
	})
	return updated, err
}

// SetAuditRetention sets (or, with nil, clears) the number of days the Audit log is
// kept (the EE advanced-retention window, ADR 0014) and records the change as a
// `workspace.update` Audit entry in the same transaction. A number of days outside the contract's
// bounds (bounds.RetentionDays) is refused. The caller has checked the license and the role. It is
// made on revision, as Update is.
func (m *Module) SetAuditRetention(ctx context.Context, s *ent.Scoped, actor events.Actor, revision int64, days *int) (*ent.Workspace, error) {
	if days != nil {
		if err := validation.Range(fieldname.RetentionDays, *days, bounds.RetentionDaysMin, bounds.RetentionDaysMax); err != nil {
			return nil, err
		}
	}
	var updated *ent.Workspace
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		before, err := editedAt(ctx, tx, s.WorkspaceID(), revision)
		if err != nil {
			return err
		}
		updated = before
		if ptrEqual(before.RetentionDays, days) {
			return nil
		}
		upd := tx.Workspace.UpdateOneID(s.WorkspaceID())
		if days == nil {
			upd = upd.ClearRetentionDays()
		} else {
			upd = upd.SetRetentionDays(*days)
		}
		if updated, err = saveAt(ctx, upd, revision); err != nil {
			return err
		}
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: s.WorkspaceID(),
			Actor:       actor,
			Action:      events.ActionWorkspaceUpdate,
			TargetType:  workspace.Label,
			TargetID:    publicid.Encode(before.ID),
			TargetName:  before.Name,
			Diff:        map[string]any{workspace.FieldRetentionDays: events.Change(before.RetentionDays, days)},
		})
	})
	return updated, err
}

// SetTimezone sets the Workspace time zone (ADR 0057) and records the change as a
// `workspace.update` Audit entry in the same transaction. A name the zone database does not
// know is refused with ErrUnknownZone. The caller has checked the role. It is made on revision, as
// Update is.
func (m *Module) SetTimezone(ctx context.Context, s *ent.Scoped, actor events.Actor, revision int64, name string) (*ent.Workspace, error) {
	if err := CheckZone(name); err != nil {
		return nil, err
	}
	var updated *ent.Workspace
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		before, err := editedAt(ctx, tx, s.WorkspaceID(), revision)
		if err != nil {
			return err
		}
		updated = before
		if before.Timezone != nil && *before.Timezone == name {
			return nil
		}
		updated, err = saveAt(ctx, tx.Workspace.UpdateOneID(s.WorkspaceID()).SetTimezone(name), revision)
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
			TargetID:    publicid.Encode(before.ID),
			TargetName:  before.Name,
			Diff:        map[string]any{workspace.FieldTimezone: events.Change(from, to)},
		})
	})
	return updated, err
}

// ErrUnknownCurrency refuses a Reporting currency code that is not in ISO 4217.
var ErrUnknownCurrency = validation.RefuseField(fieldname.ReportingCurrency, "workspaces: unknown currency code", i18n.ErrorsCurrencyUnknown)

// ErrUnknownZone refuses a Workspace or a User time zone the zone database does not know. It
// wraps reportday.ErrUnknownZone.
var ErrUnknownZone = validation.RefuseField(fieldname.Timezone, "workspaces: unknown time zone", i18n.ErrorsTimezoneUnknown)

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
// transaction (ADR 0064). A code outside ISO 4217 is refused with ErrUnknownCurrency, and a window
// outside the contract's bounds (bounds.AttributionWindowDays) too. The caller has checked the role.
// It is made on revision, as Update is.
func (m *Module) SetRevenueSettings(ctx context.Context, s *ent.Scoped, actor events.Actor, revision int64, code *string, windowDays *int) (*ent.Workspace, error) {
	if windowDays != nil {
		if err := validation.Range(fieldname.AttributionWindowDays, *windowDays,
			bounds.AttributionWindowDaysMin, bounds.AttributionWindowDaysMax); err != nil {
			return nil, err
		}
	}
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
		before, err := editedAt(ctx, tx, s.WorkspaceID(), revision)
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
		updated, err = saveAt(ctx, upd, revision)
		if err != nil {
			return err
		}
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: s.WorkspaceID(),
			Actor:       actor,
			Action:      events.ActionWorkspaceUpdate,
			TargetType:  workspace.Label,
			TargetID:    publicid.Encode(before.ID),
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
	err := lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
		n, err := m.client.Workspace.Update().
			Where(workspace.ID(s.WorkspaceID()), workspace.SetupCompletedAtIsNil()).
			SetSetupCompletedAt(now).
			Save(ctx)
		return n > 0, err
	})
	if err != nil && !errors.Is(err, lifecycle.ErrInvalidTransition) {
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
// (ADR 0105). The caller has checked the role. Switching it on is made on revision, as Update is;
// lifting it names none (nil): it is the one operation a member withheld by the requirement may
// still perform, and it takes away a demand rather than writing over anything an editor shows.
func (m *Module) SetSecondFactorRequirement(ctx context.Context, s *ent.Scoped, actor events.Actor, revision *int64, required bool, now time.Time) (updated *ent.Workspace, err error) {
	err = m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		before, err := tx.Workspace.Get(ctx, s.WorkspaceID())
		if err != nil {
			return err
		}
		if revision != nil && before.Revision != *revision {
			return ErrRevisionChanged
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
		if revision != nil {
			upd = upd.Where(workspace.Revision(*revision))
		}
		if updated, err = upd.Save(ctx); err != nil {
			if ent.IsNotFound(err) {
				return ErrRevisionChanged
			}
			return err
		}
		if err := events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: s.WorkspaceID(),
			Actor:       actor,
			Action:      events.ActionWorkspaceUpdate,
			TargetType:  workspace.Label,
			TargetID:    publicid.Encode(before.ID),
			TargetName:  before.Name,
			Diff:        map[string]any{workspace.FieldSecondFactorRequiredAt: events.Change(before.SecondFactorRequiredAt, updated.SecondFactorRequiredAt)},
		}); err != nil || !required {
			return err
		}
		return pub.Publish(ctx, &events.SecondFactorRequired{WorkspaceID: s.WorkspaceID()})
	})
	return updated, err
}

// ErrRevisionChanged means a settings edit was made on a revision the Workspace has moved past (ADR
// 0119): someone changed its settings since the client read them. Its bookkeeping (schema.Bookkeeping:
// the service feed's position, the notices of the abuse detector, the Setup checklist's completion)
// does not move it. Nothing is written.
var ErrRevisionChanged = validation.RevisionChanged("workspaces: workspace changed since it was read", i18n.ErrorsWorkspaceRevisionChanged)

// editedAt reads the Workspace a settings edit was made on, in the edit's transaction:
// ErrRevisionChanged when it has moved past revision.
func editedAt(ctx context.Context, tx *ent.Client, id, revision int64) (*ent.Workspace, error) {
	before, err := tx.Workspace.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if before.Revision != revision {
		return nil, ErrRevisionChanged
	}
	return before, nil
}

// saveAt writes upd conditional on revision, so a write committed since editedAt read it refuses
// this one rather than being overwritten.
func saveAt(ctx context.Context, upd *ent.WorkspaceUpdateOne, revision int64) (*ent.Workspace, error) {
	saved, err := upd.Where(workspace.Revision(revision)).Save(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrRevisionChanged
	}
	return saved, err
}

func ptrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// Get is the scoped Workspace's own row.
func (m *Module) Get(ctx context.Context, s *ent.Scoped) (*ent.Workspace, error) {
	return s.Workspace(ctx)
}

// ByPublicIDIn loads the Workspace a Workspace id (ADR 0043) names, live or deleted, through client
// (the raw client or the caller's transaction): an operator addresses a Workspace by its id (a
// suspension from the command line, the Staff console). An unknown id is an ent NotFound error.
func ByPublicIDIn(ctx context.Context, client *ent.Client, publicID string) (*ent.Workspace, error) {
	w, err := client.Workspace.Query().Where(workspace.PublicID(publicID)).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace %q: %w", publicID, err)
	}
	return w, nil
}

// IDByPublicIDIn is the internal key of the Workspace ByPublicIDIn loads.
func IDByPublicIDIn(ctx context.Context, client *ent.Client, publicID string) (int64, error) {
	w, err := ByPublicIDIn(ctx, client, publicID)
	if err != nil {
		return 0, err
	}
	return w.ID, nil
}

// IDByPublicID resolves a Workspace id (ADR 0043) an operator types to the internal key, whatever
// the Workspace's deletion state: the event history of a Workspace is read while it awaits
// deletion. An unknown one is ErrNotFound.
func (m *Module) IDByPublicID(ctx context.Context, publicID string) (int64, error) {
	id, err := IDByPublicIDIn(ctx, m.client, publicID)
	if ent.IsNotFound(err) {
		return 0, fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	return id, err
}

// Live is a Workspace that is not deleted (ADR 0043): every lookup that must not reach a
// deleted Workspace (its Members' scope, the lists, the Instance-wide readers) uses it, so
// "live" is stated once.
func Live() predicate.Workspace { return workspace.DiscardedAtIsNil() }

// LiveIn loads, on the transaction tx, the Workspaces among ids that are Live, in ascending id
// order.
func LiveIn(ctx context.Context, tx *ent.Client, ids ...int64) ([]*ent.Workspace, error) {
	return tx.Workspace.Query().Where(workspace.IDIn(ids...), Live()).Order(ent.Asc(workspace.FieldID)).All(ctx)
}

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
// checked the role. It is made on revision, as Update is.
func (m *Module) SetDomainJoin(ctx context.Context, s *ent.Scoped, actor events.Actor, revision int64, enabled bool) (*ent.Workspace, error) {
	var updated *ent.Workspace
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		before, err := editedAt(ctx, tx, s.WorkspaceID(), revision)
		if err != nil {
			return err
		}
		updated = before
		if before.DomainJoinEnabled == enabled {
			return nil
		}
		if updated, err = saveAt(ctx, tx.Workspace.UpdateOneID(before.ID).SetDomainJoinEnabled(enabled), revision); err != nil {
			return err
		}
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: before.ID,
			Actor:       actor,
			Action:      events.ActionWorkspaceUpdate,
			TargetType:  workspace.Label,
			TargetID:    publicid.Encode(before.ID),
			TargetName:  before.Name,
			Diff:        map[string]any{workspace.FieldDomainJoinEnabled: events.Change(before.DomainJoinEnabled, enabled)},
		})
	})
	return updated, err
}
