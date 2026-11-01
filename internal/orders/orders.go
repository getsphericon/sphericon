package orders

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/heldorderevent"
	"github.com/getsphericon/sphericon/ent/order"
	"github.com/getsphericon/sphericon/ent/ordermovement"
	"github.com/getsphericon/sphericon/internal/events"
)

// Module builds Orders and their Movements from stored order Events and credits each Order to
// the message last clicked (ADR 0064). Its functions take the Workspace-scoped client; the one
// place that scopes it is the event subscriber (docs/architecture.md).
type Module struct {
	currency CurrencyAdopter
}

// CurrencyAdopter makes a currency the Workspace's Reporting currency when it has none yet and
// records that in the Audit log (the Workspace's own writes live in the accounts package).
type CurrencyAdopter interface {
	AdoptReportingCurrency(ctx context.Context, s *ent.Scoped, code string) error
}

// New is the Orders module; currency is offered the currency of every Order Completed it reads.
func New(currency CurrencyAdopter) *Module { return &Module{currency: currency} }

var _ events.OrderRecorder = (*Module)(nil)

// IsOrderAction reports whether action names one of the four order Events.
func (m *Module) IsOrderAction(action string) bool { return IsOrderAction(action) }

// Prepare runs before the transaction of Record, on its own: the first Order Completed a
// Workspace reads makes its currency the Reporting currency while none is set. A repeat finds one
// set and changes nothing; an Event Record will not read changes nothing either.
func (m *Module) Prepare(ctx context.Context, s *ent.Scoped, in events.OrderEvent) error {
	if in.Action != ActionCompleted {
		return nil
	}
	parsed, err := Parse(in.Action, in.Properties, nil)
	if IsRefusal(err) {
		return nil // Record logs it
	}
	if err != nil {
		return err
	}
	w, err := s.Workspace(ctx)
	if err != nil {
		return fmt.Errorf("read the Workspace: %w", err)
	}
	if w.ReportingCurrency != nil {
		return nil // set already: no transaction for every Order
	}
	if err := m.currency.AdoptReportingCurrency(ctx, s, parsed.Currency); err != nil {
		return fmt.Errorf("adopt the currency of order %q: %w", parsed.OrderID, err)
	}
	return nil
}

// Record applies one order Event to the Workspace behind s, which runs inside a transaction
// that serialises the order Events of the Workspace (the events package takes the lock). An
// Order Completed creates the Order and its first Movement (a repeat of the same order_id changes
// nothing), credits it (see credit) and then applies the Events that were held for it. An
// Updated, Refunded or Cancelled adds its Movement to an Order that exists and is held until its
// Completed otherwise; one that names no currency is in the Order's own.
func (m *Module) Record(ctx context.Context, s *ent.Scoped, in events.OrderEvent) error {
	if in.Action != ActionCompleted {
		return m.adjust(ctx, s, in)
	}
	parsed, err := Parse(in.Action, in.Properties, nil)
	if IsRefusal(err) {
		// Ingest refuses these, so this Event was stored before the rule it breaks; retrying it
		// would fail the same way.
		slog.WarnContext(ctx, "order event not read", "reason", err.Error(), "event_id", in.EventID)
		return nil
	}
	if err != nil {
		return err
	}
	w, err := s.Workspace(ctx)
	if err != nil {
		return fmt.Errorf("read the Workspace: %w", err)
	}
	return m.complete(ctx, s, in, parsed, w.AttributionWindowDays)
}

func (m *Module) complete(ctx context.Context, s *ent.Scoped, in events.OrderEvent, parsed Parsed, windowDays int) error {
	create := s.Order().Create().SetOrderID(parsed.OrderID).SetCurrency(parsed.Currency)
	if in.ContactID != 0 {
		// Delivery is asynchronous, so the Contact may be gone by now: the Order is kept
		// unlinked rather than violating the contact_id foreign key.
		linked, err := s.Contact().Query().Where(contact.ID(in.ContactID)).Exist(ctx)
		if err != nil {
			return fmt.Errorf("resolve the Contact of order %q: %w", parsed.OrderID, err)
		}
		if linked {
			create.SetContactID(in.ContactID)
			if err := credit(ctx, s, create, in.ContactID, in.OccurredAt, windowDays); err != nil {
				return fmt.Errorf("find the credit of order %q: %w", parsed.OrderID, err)
			}
		}
	}
	id, err := create.OnConflictColumns(order.FieldWorkspaceID, order.FieldOrderID).DoNothing().ID(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // the same order_id again: nothing changes
	}
	if err != nil {
		return fmt.Errorf("create order %q: %w", parsed.OrderID, err)
	}
	err = s.OrderMovement().Create().
		SetOrderID(id).
		SetKind(ordermovement.KindCompleted).
		SetAmount(parsed.Total).
		SetOccurredAt(in.OccurredAt).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("record the total of order %q: %w", parsed.OrderID, err)
	}
	return m.applyHeld(ctx, s, id, parsed)
}

// applyHeld applies the Events that arrived before the Order Completed, in their own time order
// (the one stored first on equal times), and drops them. One that names no currency is in the
// Order's; one that can no longer be read is dropped with a log line.
func (m *Module) applyHeld(ctx context.Context, s *ent.Scoped, orderRef int64, parsed Parsed) error {
	held, err := s.HeldOrderEvent().Query().Where(heldorderevent.OrderID(parsed.OrderID)).
		Order(ent.Asc(heldorderevent.FieldOccurredAt), ent.Asc(heldorderevent.FieldID)).All(ctx)
	if err != nil {
		return fmt.Errorf("read the Events held for order %q: %w", parsed.OrderID, err)
	}
	for _, h := range held {
		action := actionOf(h.Kind)
		p, err := Parse(action, h.Properties, &parsed.Currency)
		if IsRefusal(err) {
			slog.WarnContext(ctx, "held order event not read", "reason", err.Error(), "event_id", h.EventID)
		} else if err != nil {
			return err
		} else if err := m.apply(ctx, s, orderRef, parsed.Currency, newChange(action, p, h.EventID, h.OccurredAt)); err != nil {
			return err
		}
		if err := s.HeldOrderEvent().DeleteOneID(h.ID).Exec(ctx); err != nil {
			return fmt.Errorf("drop a held Event of order %q: %w", parsed.OrderID, err)
		}
	}
	return nil
}

func (m *Module) adjust(ctx context.Context, s *ent.Scoped, in events.OrderEvent) error {
	orderID, _ := in.Properties[propOrderID].(string)
	found, err := s.Order().Query().Where(order.OrderID(orderID)).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return m.hold(ctx, s, in)
	}
	if err != nil {
		return fmt.Errorf("read order %q: %w", orderID, err)
	}
	parsed, err := Parse(in.Action, in.Properties, &found.Currency)
	if IsRefusal(err) {
		slog.WarnContext(ctx, "order event not read", "reason", err.Error(), "event_id", in.EventID)
		return nil
	}
	if err != nil {
		return err
	}
	return m.apply(ctx, s, found.ID, found.Currency, newChange(in.Action, parsed, in.EventID, in.OccurredAt))
}

// hold keeps an Updated, Refunded or Cancelled as sent, to be read when its Order Completed
// arrives.
func (m *Module) hold(ctx context.Context, s *ent.Scoped, in events.OrderEvent) error {
	orderID, _ := in.Properties[propOrderID].(string)
	err := s.HeldOrderEvent().Create().
		SetOrderID(orderID).
		SetKind(heldorderevent.Kind(movementKindOf(in.Action))).
		SetProperties(Read(in.Properties)).
		SetEventID(in.EventID).
		SetOccurredAt(in.OccurredAt).
		OnConflictColumns(heldorderevent.FieldWorkspaceID, heldorderevent.FieldEventID).DoNothing().Exec(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("hold an Event of order %q: %w", orderID, err)
	}
	return nil
}

// change is one order Event after the Order Completed, in the Event's own terms.
type change struct {
	kind     ordermovement.Kind
	total    *int64 // nil for an Order Cancelled that sent none
	currency string
	refundID string
	eventID  string
	at       time.Time
}

func newChange(action string, p Parsed, eventID string, at time.Time) change {
	c := change{kind: movementKindOf(action), currency: p.Currency, refundID: p.RefundID, eventID: eventID, at: at}
	if p.HasTotal {
		c.total = &p.Total
	}
	return c
}

func actionOf(k heldorderevent.Kind) string {
	switch k {
	case heldorderevent.KindUpdated:
		return ActionUpdated
	case heldorderevent.KindRefunded:
		return ActionRefunded
	default:
		return ActionCancelled
	}
}

func movementKindOf(action string) ordermovement.Kind {
	switch action {
	case ActionUpdated:
		return ordermovement.KindUpdated
	case ActionRefunded:
		return ordermovement.KindRefunded
	default:
		return ordermovement.KindCancelled
	}
}

// apply adds the Movement of one Event to the Order. The amount is worked out from the net
// total at this moment: an update adds the difference to its new total, a refund subtracts what
// it asks up to what is left and a cancellation subtracts what is left, so the net total of an
// Order never drops below zero. An Event is read once (by its id), and a refund_id refunds once.
func (m *Module) apply(ctx context.Context, s *ent.Scoped, orderRef int64, orderCurrency string, c change) error {
	if c.currency != orderCurrency {
		// One Order is one currency: an amount in another cannot be added to it.
		slog.WarnContext(ctx, "order event in another currency than its order not read", "event_id", c.eventID)
		return nil
	}
	if c.eventID != "" {
		seen, err := s.OrderMovement().Query().Where(ordermovement.EventID(c.eventID)).Exist(ctx)
		if err != nil {
			return fmt.Errorf("look for the Event %q: %w", c.eventID, err)
		}
		if seen {
			return nil
		}
	}
	if c.refundID != "" {
		seen, err := s.OrderMovement().Query().
			Where(ordermovement.OrderID(orderRef), ordermovement.RefundID(c.refundID)).Exist(ctx)
		if err != nil {
			return fmt.Errorf("look for the refund %q: %w", c.refundID, err)
		}
		if seen {
			return nil
		}
	}
	amounts, err := s.OrderMovement().Query().Where(ordermovement.OrderID(orderRef)).Select(ordermovement.FieldAmount).Ints(ctx)
	if err != nil {
		return fmt.Errorf("read the net total of an order: %w", err)
	}
	var net int64
	for _, a := range amounts {
		net += int64(a)
	}
	var total int64
	if c.total != nil {
		total = max(*c.total, 0)
	}
	var amount int64
	switch c.kind {
	case ordermovement.KindUpdated:
		amount = total - net
	case ordermovement.KindRefunded:
		amount = -min(total, max(net, 0))
	default:
		amount = -max(net, 0)
	}
	create := s.OrderMovement().Create().SetOrderID(orderRef).SetKind(c.kind).SetAmount(amount).SetOccurredAt(c.at)
	if c.eventID != "" {
		create.SetEventID(c.eventID)
	}
	if c.refundID != "" {
		create.SetRefundID(c.refundID)
	}
	if err := create.Exec(ctx); err != nil {
		return fmt.Errorf("record a %s of an order: %w", c.kind, err)
	}
	return nil
}
