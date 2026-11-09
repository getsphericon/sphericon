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

// DeleteIn discards the Integration row for good (ADR 0049) on the transaction's scoped client,
// without asking whether work still sends through it: internal/usecase/integrationdeletion runs the
// Removal check (its Live use) under the Workspace lock first. A deleted Integration gives up the
// default: its channel has none until another is picked. A missing row is integrations.ErrNotFound.
func DeleteIn(ctx context.Context, ts *ent.Scoped, row *ent.Integration, at time.Time) error {
	if row.IsDefault {
		if err := ts.Integration().UpdateOneID(row.ID).SetIsDefault(false).Exec(ctx); err != nil {
			return missing(err)
		}
	}
	return missing(ts.Integration().Discard(ctx, row.ID, at))
}

// missing turns ent's not found into integrations.ErrNotFound.
func missing(err error) error {
	if ent.IsNotFound(err) {
		return integrations.ErrNotFound
	}
	return err
}
