package orders

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/heldorderevent"
	"github.com/getsphericon/sphericon/ent/order"
	"github.com/getsphericon/sphericon/ent/ordermovement"
)

// Module builds Orders and their Movements from stored order Events (ADR 0064). Its
// functions take the Workspace-scoped client; the one place that scopes it is the event
// subscriber (docs/architecture.md).
type Module struct{}

// New is the Orders module.
func New() *Module { return &Module{} }

// Input is one order Event as the subscriber hands it over: the Event's own properties, the
// Contact its identity resolved to at ingest (0 for none) and its time.
type Input struct {
	Action     string
	Properties map[string]any
	ContactID  int64
	OccurredAt time.Time
	// EventID is the id of the bus event, so a redelivered one is read once.
	EventID string
	// FindClick looks up the click that credits an Order (see ClickFinder).
	FindClick ClickFinder
}

// Record applies one order Event to the Workspace behind s, which runs inside a transaction
// that serialises the order Events of the Workspace (the events package takes the lock). An
// Order Completed creates the Order and its first Movement (a repeat of the same order_id changes
// nothing) and then applies the Events that were held for it. An Updated, Refunded or Cancelled
// adds its Movement to an Order that exists and is held until its Completed otherwise.
//
// An Event that names no currency is in the Reporting currency, or, when it changes an Order that
// exists, in the Order's own. An Order Completed that names none while the Workspace has not
// chosen a Reporting currency is held, and Resolve reads it once one is chosen.
func (m *Module) Record(ctx context.Context, s *ent.Scoped, in Input) error {
	w, err := s.Workspace(ctx)
	if err != nil {
		return fmt.Errorf("read the Workspace: %w", err)
	}
	parsed, err := Parse(in.Action, in.Properties, w.ReportingCurrency)
	if IsRefusal(err) {
		// Ingest refuses these, so this is an Event that was valid then and is not now (the
		// Reporting currency changed meanwhile and the amount has more places than the new one
		// has). Retrying it would fail the same way.
		slog.WarnContext(ctx, "order event not read", "reason", err.Error(), "workspace_id", w.ID)
		return nil
	}
	if err != nil {
		return err
	}
	switch {
	case in.Action != ActionCompleted:
		return m.adjust(ctx, s, in)
	case parsed.Deferred:
		return m.hold(ctx, s, in)
	default:
		return m.complete(ctx, s, in, parsed, w.AttributionWindowDays)
	}
}

// Resolve reads the Order Completed Events that were held for want of a Reporting currency, in
// the order of their time, once the Workspace has chosen one; with none chosen it does nothing.
// An Event whose amount has more places than that currency allows can never be read and is
// dropped with a log line.
func (m *Module) Resolve(ctx context.Context, s *ent.Scoped, findClick ClickFinder) error {
	w, err := s.Workspace(ctx)
	if err != nil {
		return fmt.Errorf("read the Workspace: %w", err)
	}
	if w.ReportingCurrency == nil {
		return nil
	}
	held, err := s.HeldOrderEvent().Query().Where(heldorderevent.KindEQ(heldorderevent.KindCompleted)).
		Order(ent.Asc(heldorderevent.FieldOccurredAt), ent.Asc(heldorderevent.FieldID)).All(ctx)
	if err != nil {
		return fmt.Errorf("read the held Orders Completed: %w", err)
	}
	for _, h := range held {
		if err := s.HeldOrderEvent().DeleteOneID(h.ID).Exec(ctx); err != nil {
			return fmt.Errorf("drop a held Order Completed: %w", err)
		}
		in := Input{
			Action: ActionCompleted, Properties: h.Properties, ContactID: lo.FromPtr(h.ContactID),
			OccurredAt: h.OccurredAt, EventID: h.EventID, FindClick: findClick,
		}
		parsed, err := Parse(in.Action, in.Properties, w.ReportingCurrency)
		if IsRefusal(err) {
			slog.WarnContext(ctx, "held order event not read", "reason", err.Error(), "workspace_id", w.ID)
			continue
		}
		if err != nil {
			return err
		}
		if err := m.complete(ctx, s, in, parsed, w.AttributionWindowDays); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) complete(ctx context.Context, s *ent.Scoped, in Input, parsed Parsed, windowDays int) error {
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
			if err := credit(ctx, s, create, in, windowDays); err != nil {
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
	held, err := s.HeldOrderEvent().Query().Where(heldorderevent.OrderID(parsed.OrderID), heldorderevent.KindNEQ(heldorderevent.KindCompleted)).
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

func (m *Module) adjust(ctx context.Context, s *ent.Scoped, in Input) error {
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

// hold keeps the Event as sent, to be read when its Order Completed arrives (or, for an Order
// Completed, when a Reporting currency is chosen).
func (m *Module) hold(ctx context.Context, s *ent.Scoped, in Input) error {
	orderID, _ := in.Properties[propOrderID].(string)
	kind := heldorderevent.Kind(movementKindOf(in.Action))
	if in.Action == ActionCompleted {
		kind = heldorderevent.KindCompleted
	}
	create := s.HeldOrderEvent().Create().
		SetOrderID(orderID).
		SetKind(kind).
		SetProperties(Read(in.Properties)).
		SetEventID(in.EventID).
		SetOccurredAt(in.OccurredAt)
	if in.Action == ActionCompleted && in.ContactID != 0 {
		create.SetContactID(in.ContactID)
	}
	err := create.OnConflictColumns(heldorderevent.FieldWorkspaceID, heldorderevent.FieldEventID).DoNothing().Exec(ctx)
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

// Click is the click that earns an Order its credit: the Broadcast or the Automation (one is
// set) whose email was clicked, and when.
type Click struct {
	BroadcastID  int64
	AutomationID int64
	At           time.Time
}

// ClickFinder finds the latest human click of the Contact on a Broadcast's or an Automation's
// email in [since, until], nil for none. The events package supplies it: it reads the Events, and
// this package cannot import it.
type ClickFinder func(ctx context.Context, s *ent.Scoped, contactID int64, since, until time.Time) (*Click, error)

// credit sets on create the Broadcast or Automation of the latest click of the Contact before the
// order's time and inside the Attribution window (ADR 0064). It is looked for once, here: a repeat
// of the Order Completed never reaches it, so a later change of the window or a click stored
// afterwards, though dated before the order, changes nothing.
func credit(ctx context.Context, s *ent.Scoped, create *ent.OrderScopedCreate, in Input, windowDays int) error {
	if in.FindClick == nil {
		return nil
	}
	click, err := in.FindClick(ctx, s, in.ContactID, in.OccurredAt.AddDate(0, 0, -windowDays), in.OccurredAt)
	if err != nil || click == nil {
		return err
	}
	switch {
	case click.BroadcastID != 0:
		create.SetBroadcastID(click.BroadcastID)
	case click.AutomationID != 0:
		create.SetAutomationID(click.AutomationID)
	default:
		return nil
	}
	create.SetCreditedClickAt(click.At)
	return nil
}
