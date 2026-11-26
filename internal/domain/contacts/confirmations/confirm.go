package confirmations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/contacts/consentstamps"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/emailcase"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
)

// RecordConfirmation writes the derived Confirmation read-model row (provenance
// double_opt_in) and publishes the immutable marketing.confirmed Event in one
// transaction (ADR 0013) — the positive mirror of the unsubscribe link. The click on an unexpired
// link is the person's own act on fresh proof, so before the Confirmation it lifts the address's
// opt-out of everything (liftEverything); a per-source opt-out stays. It is keyed
// by destination, so it records even if the contact was deleted between send and
// click. A repeated POST (mailbox retry, double click) is a complete no-op: the
// confirmation stands and no second Event is logged. s is the scope of the token's Workspace,
// which the entry point builds from it; the writes go through it as the ingest actor (never
// audited), since no membership or API token stands behind them. The link is followed now, by the
// module's clock.
func (m *Module) RecordConfirmation(ctx context.Context, s *ent.Scoped, target linktarget.Confirmation, ip string) error {
	dest := emailcase.Of(target.Destination)
	if dest == "" || target.WorkspaceID == 0 {
		return nil
	}

	// The Workspace comes from the signed confirmation token (no membership or api
	// token here), so this is a scope source outside the site/external/job list.
	at := m.now()
	return m.bus.WithinScopedTx(ctx, events.Ingest(s), func(sc *ent.Scoped, _ events.Publisher) error {
		// A link of an erased Workspace has nothing left to record into.
		ws, err := sc.Workspace(ctx)
		if err != nil || workspaces.DeletionStateOf(ws) == workspaces.StateErased {
			return err
		}
		if err := m.liftEverything(ctx, sc, target.ContactID, dest, at); err != nil {
			return err
		}
		_, _, err = m.recordIn(ctx, sc, grant{
			channel: channels.Email, destination: dest, provenance: confirmation.ProvenanceDoubleOptIn,
			contactID: target.ContactID, ip: ip, at: consentstamps.At(at),
		})
		if errors.Is(err, ErrSuperseded) {
			// The address left after the link was followed: the click changed nothing, and is in the
			// Contact's history as such (ADR 0154).
			return m.publishSuperseded(ctx, sc, target.ContactID, dest, at)
		}
		return err
	})
}

// liftEverything lifts the opt-out of everything of address, the person returning by the link at
// at, and records the return as email.resubscribed for the Contact the link was issued to (one gone
// by now has nobody to name), ordered with the opt-out by when each occurred (ADR 0154). Nothing to
// lift records nothing, so a repeated click is a no-op.
func (m *Module) liftEverything(ctx context.Context, s *ent.Scoped, contactID int64, address string, at time.Time) error {
	lifted, err := m.unsubs.LiftEmail(ctx, s, address, consentstamps.At(at))
	if err != nil || !lifted {
		return err
	}
	holder, err := m.locks.Referenced(ctx, s, contactID)
	if err != nil {
		return fmt.Errorf("lock the returning contact: %w", err)
	}
	return m.bus.PublishIn(ctx, s, &events.EmailResubscribed{
		WorkspaceID: s.WorkspaceID(), ContactID: holder, Email: address,
		Source: eligibility.SourceEverything, Via: events.ResubscribedViaConfirmation, ObservedAt: at,
	})
}

// publishSuperseded records a double opt-in click older than the address's withdrawal as
// contact.consent_superseded for the Contact the link was issued to; one gone by now has nobody to
// name.
func (m *Module) publishSuperseded(ctx context.Context, s *ent.Scoped, contactID int64, address string, at time.Time) error {
	if contactID == 0 {
		return nil
	}
	exists, err := s.Contact().Query().Where(contact.ID(contactID)).Exist(ctx)
	if err != nil || !exists {
		return err
	}
	return m.bus.PublishIn(ctx, s, &events.ConsentSuperseded{
		WorkspaceID: s.WorkspaceID(), ContactID: contactID, Email: address,
		Channel: string(channels.Email), Observation: events.ObservationStart, ObservedAt: at,
	})
}
