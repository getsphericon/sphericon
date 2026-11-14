package suspension

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/defaultsendcap"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// defaultID is the id of the one row of the default Send cap.
const defaultID int64 = 1

// ErrNoSendCaps refuses a change of the default Send cap on an Instance that has no Send caps.
var ErrNoSendCaps = validation.NotFound("suspension: this instance has no send caps", i18n.ErrorsSendCapsAbsent)

// DefaultSendCap is the default Send cap (ADR 0067, ADR 0146): the Send cap every Workspace
// created on an Instance with Send caps starts with, a rule of the Workspace's standing kept beside
// the Send cap itself. Its one row belongs to no Workspace, so it holds the raw ent client.
type DefaultSendCap struct {
	ent *ent.Client
	// sendCaps is whether the Instance has Send caps at all (the wiring reads it from the Mode):
	// without them there is no default to show or change, and a new Workspace starts with none,
	// whatever default is stored.
	sendCaps bool
}

// NewDefaultSendCap is the default Send cap over client, on an Instance that has Send caps or not.
func NewDefaultSendCap(client *ent.Client, sendCaps bool) *DefaultSendCap {
	return &DefaultSendCap{ent: client, sendCaps: sendCaps}
}

// Get is the default as it stands: perDay messages per rolling 24 hours, nil when none is set, and
// whether the Instance has Send caps at all (ok); without them there is no default, and perDay is
// nil whatever is stored.
func (d *DefaultSendCap) Get(ctx context.Context) (perDay *int, ok bool, err error) {
	if !d.sendCaps {
		return nil, false, nil
	}
	perDay, err = storedIn(ctx, d.ent)
	return perDay, err == nil, err
}

// StartingIn is the Send cap a Workspace created now starts with, read on tx, the transaction
// that creates it: on an Instance with Send caps the stored default, nil when none is set; on one
// without them, none, whatever default is stored.
func (d *DefaultSendCap) StartingIn(ctx context.Context, tx *ent.Client) (*int, error) {
	if !d.sendCaps {
		return nil, nil
	}
	return storedIn(ctx, tx)
}

// SetIn sets the default to perDay messages per rolling 24 hours, or clears it with nil, on tx,
// and reports the change as an Audit diff value (events.ChangeOf) and whether the default changed:
// setting the same value or clearing none is a no-op. The row is locked, so two changes at once
// each see the value the other left. A default outside the contract's bounds (bounds.MaxPerDay) is
// refused, and so is any change on an Instance without Send caps (ErrNoSendCaps). Existing
// Workspaces keep their Send caps. The caller records the change in its Audit log, in the same
// transaction.
func (d *DefaultSendCap) SetIn(ctx context.Context, tx *ent.Client, perDay *int) (change map[string]any, changed bool, err error) {
	if !d.sendCaps {
		return nil, false, ErrNoSendCaps
	}
	if perDay != nil {
		if err := validation.Range(fieldname.PerDay, *perDay, bounds.MaxPerDayMin, bounds.MaxPerDayMax); err != nil {
			return nil, false, err
		}
	}
	err = tx.DefaultSendCap.Create().SetID(defaultID).OnConflictColumns(defaultsendcap.FieldID).DoNothing().Exec(ctx)
	if _, err := db.Inserted(err); err != nil {
		return nil, false, fmt.Errorf("set the default send cap: %w", err)
	}
	row, err := tx.DefaultSendCap.Query().Where(defaultsendcap.ID(defaultID)).ForUpdate().Only(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("set the default send cap: %w", err)
	}
	change, differs := events.ChangeOf(row.PerDay, perDay)
	if !differs {
		return nil, false, nil
	}
	upd := tx.DefaultSendCap.UpdateOneID(defaultID)
	if perDay == nil {
		upd.ClearPerDay()
	} else {
		upd.SetPerDay(*perDay)
	}
	if err := upd.Exec(ctx); err != nil {
		return nil, false, fmt.Errorf("set the default send cap: %w", err)
	}
	return change, true, nil
}

// storedIn is the stored default read on client, nil when none is set.
func storedIn(ctx context.Context, client *ent.Client) (*int, error) {
	row, err := client.DefaultSendCap.Query().Where(defaultsendcap.ID(defaultID)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the default send cap: %w", err)
	}
	return row.PerDay, nil
}
