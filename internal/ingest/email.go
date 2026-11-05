package ingest

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/preset"
)

// email reads Delivery notices; email has no Inbound messages or Reachability changes.
type email struct{ Ignore }

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
	msg, err := outbound.FindByProviderMessageID(ctx, tx.Scoped, n.MessageID)
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
	if err := tx.Pub.Publish(ctx, ev); err != nil {
		return err
	}
	if msg == nil {
		return nil
	}
	if result, ok := resultOf(ev.Action); ok {
		return outbound.StampDelivery(ctx, tx.Scoped, msg.ID, result, tx.Now)
	}
	return nil
}

// resultOf is the delivery result a notice's Event stamps on its Outbound message. Delay and
// drop stamp nothing (ADR 0053).
func resultOf(action string) (outbound.DeliveryResult, bool) {
	switch action {
	case events.NameEmailDelivered:
		return outbound.ResultDelivered, true
	case events.NameEmailBounced:
		return outbound.ResultBounced, true
	case events.NameEmailComplained:
		return outbound.ResultComplained, true
	}
	return "", false
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

// RecordOutcome counts a provider's bounce or complaint on the send-outcome counter. The bus runs
// it for every Event newly stored, once the Event's transaction committed (events.Bus.OnStored),
// so neither a repeated notice nor a rolled-back one counts.
func RecordOutcome(ctx context.Context, ev events.DomainEvent) {
	f, ok := ev.(*events.EmailDelivery)
	if !ok || f.Provider == "" {
		return
	}
	var status messaging.SendStatus
	switch f.Action {
	case events.NameEmailBounced:
		status = messaging.SendBounce
	case events.NameEmailComplained:
		status = messaging.SendComplaint
	default:
		return
	}
	// Provider is the Preset's name (ADR 0037), or "ses" for the SES adapter.
	messaging.RecordSendOutcome(ctx, providers.Provider(f.Provider), status)
}
