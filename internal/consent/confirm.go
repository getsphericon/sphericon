package consent

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/suppression"
)

// RecordConfirmation writes the derived Confirmation read-model row (provenance
// double_opt_in) and publishes the immutable marketing.confirmed Event in one
// transaction (ADR 0013) — the positive mirror of RecordUnsubscribe. It is keyed
// by destination, so it records even if the contact was deleted between send and
// click. A repeated POST (mailbox retry, double click) is a complete no-op: the
// confirmation stands and no second Event is logged.
func RecordConfirmation(ctx context.Context, bus *events.Bus, target linktarget.Confirmation, ip string) error {
	dest := suppression.NormalizeDestination(target.Destination)
	if dest == "" || target.WorkspaceID == 0 {
		return nil
	}

	return settled(bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		// The Workspace comes from the signed confirmation token (no membership or api
		// token here), so this is a scope source outside the site/external/job list.
		sc := events.Ingest(tx.Scoped(target.WorkspaceID))
		_, err := recordIn(ctx, sc, pub, grant{
			channel: channels.Email, destination: dest, provenance: confirmation.ProvenanceDoubleOptIn,
			contactID: target.ContactID, ip: ip,
		})
		return err
	}))
}
