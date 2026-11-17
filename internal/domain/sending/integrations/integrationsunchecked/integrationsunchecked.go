// Package integrationsunchecked holds the integrations module's unchecked removal (ADR 0110, ADR
// 0145): whether work still sends through an Integration is the Live use of the Broadcasts and
// Automations, modules above integrations, so that check is the use case
// internal/usecase/integrationdeletion's, the one kind of package easement lets import this one.
// Every rule of the Integration's own row stays here, the release of a Telegram bot's webhook after
// the commit included. The integrations module does not import it; the use case builds its Module.
package integrationsunchecked

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// Module is the integrations module's unchecked removal: it opens a Deletion on the Integration's
// row and applies it once the use case's check has passed (docs/architecture/modules.md, the
// unchecked operations).
type Module struct {
	bus *events.Bus
	now func() time.Time
}

// New builds the unchecked removal over the bus the release of a Telegram bot's webhook is queued
// on, in the transaction of the discard; now is the Instance clock a discard is stamped by.
func New(bus *events.Bus, now func() time.Time) *Module { return &Module{bus: bus, now: now} }

// Deletion is the plan of the discard of one Integration: the row DeletionIn read under the
// Workspace's lock (ADR 0108), so that the use case runs the Removal check (its Live use) between
// opening and applying it. A promotion to default and a change of config take the lock too, so the
// Integration applied is the one opened.
type Deletion struct {
	row *ent.Integration
}

// DeletionIn opens the deletion of the Integration with the id in the transaction l holds the
// Workspace's lock in. An unknown id is integrations.ErrNotFound; a deleted Integration opens a
// deletion that is Done.
func (m *Module) DeletionIn(ctx context.Context, l *ent.WorkspaceLocked, id int64) (*Deletion, error) {
	row, err := l.Integration().Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, integrations.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &Deletion{row: row}, nil
}

// Done reports that the Integration is deleted already: deleting it again changes nothing, and
// there is no Live use to check.
func (d *Deletion) Done() bool { return d.row.DiscardedAt != nil }

// ApplyDeletion discards the Integration d was opened on for good, now (ADR 0049), without asking
// whether work still sends through it: that is the use case's Removal check, run before in the
// transaction l holds the lock in, the one d was opened in. It gives up the default in the
// statement that discards it (schema.GivenUpOnDiscard): its channel has none until another is
// picked. A Telegram bot that held its webhook lets go of it once the discard commits: the release
// is a job queued in the transaction l is bound to, which must be one of the module's bus
// (events.Bus.EnqueueIn). A deletion that is Done writes and queues nothing.
func (m *Module) ApplyDeletion(ctx context.Context, l *ent.WorkspaceLocked, d *Deletion) error {
	if d.Done() {
		return nil
	}
	if err := l.Integration().Discard(ctx, d.row.ID, m.now()); err != nil {
		return err
	}
	if !integrations.HoldsWebhook(d.row) {
		return nil
	}
	release := integrations.ReleaseWebhookArgs{WorkspaceID: d.row.WorkspaceID, IntegrationID: d.row.ID}
	return m.bus.EnqueueIn(ctx, l, release, nil)
}
