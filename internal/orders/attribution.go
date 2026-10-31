package orders

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/events"
)

// Attribution is the last click (ADR 0064): an Order is credited to the Broadcast or the
// Automation whose email its Contact clicked last before the order, inside the Workspace's
// Attribution window. This file is the whole of it: which click counts, the window and the credit.

// click is the click that earns an Order its credit: the Broadcast or the Automation (one is
// set) whose email was clicked, and when.
type click struct {
	broadcastID  int64
	automationID int64
	at           time.Time
}

// credit sets on create the Broadcast or Automation of the latest click of the Contact before the
// order's time and inside the Attribution window of windowDays. It is looked for once, here: a
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
	create.SetCreditedClickAt(c.at)
	return nil
}

// lastClick is the latest human `email.clicked` Event of the Contact within [since, until] that
// carries a Broadcast or an Automation; nil for none. A click flagged `machine` (ADR 0053) and a
// click of neither (a Transactional send) are passed over; equal times go to the click Event
// stored last.
func lastClick(ctx context.Context, s *ent.Scoped, contactID int64, since, until time.Time) (*click, error) {
	e, err := s.Event().Query().
		Where(
			event.ContactID(contactID), event.ActionEQ(events.NameEmailClicked), events.HumanOnly(),
			event.Or(event.BroadcastIDNotNil(), event.AutomationIDNotNil()),
			event.OccurredAtGTE(since), event.OccurredAtLTE(until),
		).
		// Latest first, ties to the Event stored last.
		Order(ent.Desc(event.FieldOccurredAt), ent.Desc(event.FieldID)).
		First(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &click{broadcastID: lo.FromPtr(e.BroadcastID), automationID: lo.FromPtr(e.AutomationID), at: e.OccurredAt}, nil
}
