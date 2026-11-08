// Package sendingdomainsunchecked holds the sendingdomains module's unchecked removal (ADR 0110):
// whether work still sends from a Sending domain is the Live use of the Broadcasts and Automations,
// modules above sendingdomains, so the check is the use case internal/usecase/sendingdomaindeletion's,
// the one kind of package easement lets import this one. The sendingdomains module does not import
// it.
package sendingdomainsunchecked

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
)

// DeleteIn discards the Sending domain row for good (ADR 0049) on the transaction's scoped client,
// without asking whether work still sends from it: internal/usecase/sendingdomaindeletion runs the
// Removal check (its Live use) under the Workspace lock first. The record of mail sent from it
// stays.
func DeleteIn(ctx context.Context, ts *ent.Scoped, id int64, at time.Time) error {
	return ts.SendingDomain().Discard(ctx, id, at)
}
