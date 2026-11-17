// Package sendingdomainsunchecked holds the sendingdomains module's unchecked removal (ADR 0110,
// ADR 0145): whether work still sends from a Sending domain is the Live use of the Broadcasts and
// Automations, modules above sendingdomains, so that check is the use case
// internal/usecase/sendingdomaindeletion's, the one kind of package easement lets import this one.
// Every rule of the Sending domain's own row stays here. The sendingdomains module does not import
// it.
package sendingdomainsunchecked

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
)

// Module is the sendingdomains module's unchecked removal: it opens a Deletion on the Sending
// domain's row and applies it once the use case's check has passed (docs/architecture/modules.md,
// the unchecked operations).
type Module struct {
	now func() time.Time
}

// New builds the unchecked removal; now is the Instance clock a discard is stamped by.
func New(now func() time.Time) *Module { return &Module{now: now} }

// Deletion is the plan of the discard of one Sending domain: the row DeletionIn read under the
// Workspace's lock (ADR 0108), so that the use case runs the Removal check (its Live use) between
// opening and applying it, on the domain the deletion read.
type Deletion struct {
	row *ent.SendingDomain
}

// DeletionIn opens the deletion of the Sending domain with the id in the transaction l holds the
// Workspace's lock in. An unknown id is sendingdomains.ErrNotFound; a deleted domain opens a
// deletion that is Done.
func (m *Module) DeletionIn(ctx context.Context, l *ent.WorkspaceLocked, id int64) (*Deletion, error) {
	row, err := l.SendingDomain().Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, sendingdomains.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &Deletion{row: row}, nil
}

// Domain is the domain the Sending domain is, which the Removal check looks for in the From of
// the work that sends.
func (d *Deletion) Domain() string { return d.row.Domain }

// Done reports that the Sending domain is deleted already: deleting it again changes nothing, and
// there is no Live use to check.
func (d *Deletion) Done() bool { return d.row.DiscardedAt != nil }

// ApplyDeletion discards the Sending domain d was opened on for good, now (ADR 0049), without
// asking whether work still sends from it: that is the use case's Removal check, run before in the
// transaction l holds the lock in, the one d was opened in. The record of mail sent from it stays.
// A deletion that is Done writes nothing.
func (m *Module) ApplyDeletion(ctx context.Context, l *ent.WorkspaceLocked, d *Deletion) error {
	if d.Done() {
		return nil
	}
	return l.SendingDomain().Discard(ctx, d.row.ID, m.now())
}
