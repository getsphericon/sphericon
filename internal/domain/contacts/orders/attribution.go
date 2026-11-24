package orders

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/order"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// Attribution is the last click (ADR 0064, 0083): an Order is credited to the Broadcast or the
// Automation whose message its Contact clicked last, on any Channel, before the order, inside the
// Workspace's Attribution window. This file is the whole of it: which click counts, the window and the credit.

// click is the click that earns an Order its credit: the Broadcast or the Automation (one is
// set) whose message was clicked, that Outbound message, and when.
type click struct {
	broadcastID  int64
	automationID int64
	messageID    int64
	at           time.Time
}

// credit sets on create the Broadcast or Automation of the latest click of the Contact before the
// order's time and inside the Attribution window of windowDays, with the message clicked and the
// time of the click. It is looked for once, here: a
// repeat of the Order Completed never reaches it, so a later change of the window or a click stored
// afterwards, though dated before the order, changes nothing.
func credit(ctx context.Context, s *ent.Scoped, create *ent.OrderScopedCreate, contactID int64, orderedAt time.Time, windowDays int) error {
	c, err := lastClick(ctx, s, contactID, orderedAt.AddDate(0, 0, -windowDays), orderedAt)
	if err != nil || c == nil {
		return err
	}
	switch {
	case c.broadcastID != 0:
		create.SetBroadcastID(c.broadcastID)
	case c.automationID != 0:
		create.SetAutomationID(c.automationID)
	default:
		return nil
	}
	create.SetCreditedClickAt(c.at).SetCreditedMessageID(c.messageID)
	return nil
}

// lastClick is the latest human click Event (`email.clicked`, `telegram.clicked`, `sms.clicked`) of
// the Contact within [since, until] that carries a Broadcast or an Automation and names its
// Outbound message, among the messages whose tracking was not withdrawn (ADR 0120); nil for none. A
// click flagged `machine` (ADR 0053), a click of neither (a Transactional send) and a click of no
// message are passed over; equal times go to the click Event stored last.
//
// The clicked message is read FOR SHARE in the Order's transaction, so a withdrawal of its tracking
// (which takes the message's row for its update) either commits first, and the click is passed over
// for the one before it, or waits for the Order and then clears its credit. A message that is gone
// (message retention, ADR 0143) was not withdrawn: its kept click still credits.
func lastClick(ctx context.Context, s *ent.Scoped, contactID int64, since, until time.Time) (*click, error) {
	var withdrawn []int64
	for {
		which := []predicate.Event{
			event.ContactID(contactID), event.ActionIn(events.ClickActions...), events.HumanOnly(),
			event.Or(event.BroadcastIDNotNil(), event.AutomationIDNotNil()), events.NamesMessage(),
			event.OccurredAtGTE(since), event.OccurredAtLTE(until),
		}
		if len(withdrawn) > 0 {
			which = append(which, event.Not(events.NamesMessageIn(withdrawn)))
		}
		e, err := s.Event().Query().
			Where(which...).
			// Latest first, ties to the Event stored last.
			Order(ent.Desc(event.FieldOccurredAt), ent.Desc(event.FieldID)).
			First(ctx)
		if ent.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		messageID, ok := events.ClickedMessageID(e)
		if !ok {
			return nil, nil // a key with no id of a message, which the recorder never writes: nothing to credit
		}
		gone, err := isWithdrawn(ctx, s, messageID)
		if err != nil {
			return nil, err
		}
		if !gone {
			return &click{broadcastID: lo.FromPtr(e.BroadcastID), automationID: lo.FromPtr(e.AutomationID), messageID: messageID, at: e.OccurredAt}, nil
		}
		withdrawn = append(withdrawn, messageID)
	}
}

// isWithdrawn reads whether the tracking of the Outbound message id was withdrawn, holding its row
// FOR SHARE until the Order's transaction ends; a message that is gone was not. The Orders module
// is of the contacts context, below Outbound send, so it reads the Latch itself rather than ask the
// module that writes it.
func isWithdrawn(ctx context.Context, s *ent.Scoped, id int64) (bool, error) {
	msgs, err := s.OutboundMessage().Query().
		Where(outboundmessage.ID(id)).
		Select(outboundmessage.FieldID, outboundmessage.FieldTrackingWithdrawnAt).
		ForShare().
		All(ctx)
	if err != nil {
		return false, fmt.Errorf("read the clicked message: %w", err)
	}
	return len(msgs) == 1 && msgs[0].TrackingWithdrawnAt != nil, nil
}

// ClearCredits takes the credit from every Order credited through a click of one of the Outbound
// messages messageIDs names: a refusal of tracking by their Destination leaves no credit its clicks
// earned (ADR 0120). The Broadcast or Automation, the click's time and its message are cleared
// together, as the credit's check requires, and the Order is not credited again: the credit is
// looked for once. Run it in the transaction that holds the messages' rows, so a credit found
// meanwhile waits for it and finds the message withdrawn.
func (m *Module) ClearCredits(ctx context.Context, s *ent.Scoped, messageIDs []int64) error {
	if len(messageIDs) == 0 {
		return nil
	}
	_, err := s.Order().Update().
		Where(order.CreditedMessageIDIn(messageIDs...)).
		ClearBroadcastID().
		ClearAutomationID().
		ClearCreditedClickAt().
		ClearCreditedMessageID().
		Save(ctx)
	if err != nil {
		return fmt.Errorf("clear the credits of withdrawn messages: %w", err)
	}
	return nil
}
