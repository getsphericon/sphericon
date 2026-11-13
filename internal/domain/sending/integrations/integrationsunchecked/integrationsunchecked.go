// Package integrationsunchecked holds the integrations module's unchecked removal (ADR 0110, ADR
// 0145): whether work still sends through an Integration is the Live use of the Broadcasts and
// Automations, modules above integrations, so that check is the use case
// internal/usecase/integrationdeletion's, the one kind of package easement lets import this one.
// Every rule of the Integration's own row stays here, the release of a Telegram bot's webhook after
// the commit included. The integrations module does not import it.
package integrationsunchecked

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// Deletion is the discard of one Integration, opened on its row under the Workspace's lock (ADR
// 0108) so that the use case runs the Removal check (its Live use) between opening and applying
// it. A promotion to default and a change of config take the lock too, so the Integration applied
// is the one opened.
type Deletion struct {
	l   *ent.WorkspaceLocked
	row *ent.Integration
}

// DeletionIn opens the deletion of the Integration with the id in the transaction l holds the
// Workspace's lock in. An unknown id is integrations.ErrNotFound; a deleted Integration opens a
// deletion that is Done.
func DeletionIn(ctx context.Context, l *ent.WorkspaceLocked, id int64) (*Deletion, error) {
	row, err := l.Integration().Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, integrations.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &Deletion{l: l, row: row}, nil
}

// Done reports that the Integration is deleted already: deleting it again changes nothing, and
// there is no Live use to check.
func (d *Deletion) Done() bool { return d.row.DiscardedAt != nil }

// Apply discards the Integration for good at `at` (ADR 0049), without asking whether work still
// sends through it: that is the use case's Removal check, run under the same lock before. It gives
// up the default in the statement that discards it (schema.GivenUpOnDiscard): its channel has none
// until another is picked. A Telegram bot that held its webhook lets go of it once the discard
// commits: the release is a job queued on q, the transaction of the discard. A deletion that is
// Done writes and queues nothing.
func (d *Deletion) Apply(ctx context.Context, at time.Time, q events.Enqueuer) error {
	if d.Done() {
		return nil
	}
	if err := d.l.Integration().Discard(ctx, d.row.ID, at); err != nil {
		return err
	}
	if !integrations.HoldsWebhook(d.row) {
		return nil
	}
	release := integrations.ReleaseWebhookArgs{WorkspaceID: d.row.WorkspaceID, IntegrationID: d.row.ID}
	return q.Enqueue(ctx, release, nil)
}
