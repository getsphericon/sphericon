package callbackinterpretation

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// email reads Delivery notices; email has no Inbound messages or Reachability changes.
type email struct{ ignore }

// DeliveryNotice publishes the notice as an email delivery Event, linked to the Contact with
// the address and to the Outbound message the provider's message id names, and stamps that
// message's delivery result. A redelivery of the same provider event dedupes downstream on its
// DedupID (the provider's event id plus the Destination), and a stamp is set only while it is
// empty, so it never changes. Every kind is published; only a hard bounce and a complaint enter
// Suppression, which is the suppression consumer's rule.
func (email) DeliveryNotice(ctx context.Context, tx Tx, n preset.DeliveryNotice) error {
	address := strings.ToLower(strings.TrimSpace(n.Destination))
	if address == "" {
		return nil
	}
	var contactID int64
	c, err := tx.Scoped.Contact().Query().Where(contact.EmailEQ(address)).Only(ctx)
	switch {
	case err == nil:
		contactID = c.ID
	case ent.IsNotFound(err):
		// An address that is no Contact is suppressed anyway, with no Contact link.
	default:
		return fmt.Errorf("lookup contact %q: %w", address, err)
	}
	ev := &events.EmailDelivery{
		ObservedAt:        tx.Now,
		Action:            actionOf(n.Kind),
		WorkspaceID:       tx.Scoped.WorkspaceID(),
		ContactID:         contactID,
		Email:             address,
		Provider:          tx.Preset,
		SendingDomain:     n.SendingDomain,
		Reason:            n.Reason,
		ProviderMessageID: n.MessageID,
		DedupID:           n.ID + "/" + address,
	}
	if n.Kind == preset.Bounce {
		ev.BounceKind = events.BounceKindSoft
		if n.Hard {
			ev.BounceKind = events.BounceKindHard
		}
	}
	msg, err := outboundmessages.FindByProviderMessageID(ctx, tx.Scoped, n.MessageID)
	if err != nil {
		return err
	}
	if msg != nil {
		ev.OutboundMessageID = msg.ID
		ev.Surface = string(msg.Kind)
		ev.AutomationID = lo.FromPtr(msg.AutomationID)
		ev.BroadcastID = lo.FromPtr(msg.BroadcastID)
		ev.SendingSource = lo.FromPtr(msg.SendingSource)
		if ev.SendingDomain == "" {
			ev.SendingDomain = lo.FromPtr(msg.SendingDomain)
		}
		if ev.ContactID == 0 {
			ev.ContactID = lo.FromPtr(msg.ContactID)
		}
	}
	return tx.Pub.Publish(ctx, ev)
}

// actionOf is the Event action a Delivery notice kind becomes.
func actionOf(k preset.Kind) string {
	switch k {
	case preset.Delivery:
		return events.NameEmailDelivered
	case preset.Delay:
		return events.NameEmailDelayed
	case preset.Drop:
		return events.NameEmailDropped
	case preset.Complaint:
		return events.NameEmailComplained
	default:
		return events.NameEmailBounced
	}
}
