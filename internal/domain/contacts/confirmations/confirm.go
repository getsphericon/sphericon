package confirmations

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/emailcase"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
)

// RecordConfirmation writes the derived Confirmation read-model row (provenance
// double_opt_in) and publishes the immutable marketing.confirmed Event in one
// transaction (ADR 0013) — the positive mirror of the unsubscribe link. It is keyed
// by destination, so it records even if the contact was deleted between send and
// click. A repeated POST (mailbox retry, double click) is a complete no-op: the
// confirmation stands and no second Event is logged. s is the scope of the token's Workspace,
// which the entry point builds from it; the writes go through it as the ingest actor (never
// audited), since no membership or API token stands behind them.
func RecordConfirmation(ctx context.Context, bus *events.Bus, s *ent.Scoped, target linktarget.Confirmation, ip string) error {
	dest := emailcase.Of(target.Destination)
	if dest == "" || target.WorkspaceID == 0 {
		return nil
	}

	// The Workspace comes from the signed confirmation token (no membership or api
	// token here), so this is a scope source outside the site/external/job list.
	return bus.WithinScopedTx(ctx, events.Ingest(s), func(sc *ent.Scoped, pub events.Publisher) error {
		// A link of an erased Workspace has nothing left to record into.
		erased, err := workspaces.ErasedIn(ctx, sc)
		if err != nil || erased {
			return err
		}
		_, err = recordIn(ctx, sc, pub, grant{
			channel: channels.Email, destination: dest, provenance: confirmation.ProvenanceDoubleOptIn,
			contactID: target.ContactID, ip: ip,
		})
		return err
	})
}
