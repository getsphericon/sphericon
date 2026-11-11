// Package integrationsunchecked holds the integrations module's unchecked removal (ADR 0110):
// whether work still sends through an Integration is the Live use of the Broadcasts and Automations,
// modules above integrations, so the check is the use case internal/usecase/integrationdeletion's,
// the one kind of package easement lets import this one. The integrations module does not import it.
package integrationsunchecked

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
)

// DeleteIn discards the Integration with the id for good (ADR 0049) in the transaction l holds the
// Workspace's lock in (ADR 0108), without asking whether work still sends through it:
// internal/usecase/integrationdeletion runs the Removal check (its Live use) under the same lock
// first. A deleted Integration gives up the default in the statement that discards it
// (schema.GivenUpOnDiscard): its channel has none until another is picked. It returns the row as
// discarded. An unknown id, or an Integration deleted already, is integrations.ErrNotFound.
func DeleteIn(ctx context.Context, l *ent.WorkspaceLocked, id int64, at time.Time) (*ent.Integration, error) {
	if err := l.Integration().Discard(ctx, id, at); err != nil {
		return nil, missing(err)
	}
	row, err := l.Integration().Get(ctx, id)
	return row, missing(err)
}

// missing turns ent's not found into integrations.ErrNotFound.
func missing(err error) error {
	if ent.IsNotFound(err) {
		return integrations.ErrNotFound
	}
	return err
}
