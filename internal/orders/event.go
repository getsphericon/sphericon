// Package orders reads the order Events of a Workspace's shop into Orders (ADR 0064). An order
// Event keeps the names of the RudderStack E-Commerce Events spec; this package checks its
// amounts at ingest and builds the Order and its Movements from it once it is stored.
package orders

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"

	"github.com/getsphericon/sphericon/internal/money"
)

// The actions of the four order Events, as the spec names them.
const (
	ActionCompleted = "Order Completed"
	ActionUpdated   = "Order Updated"
	ActionRefunded  = "Order Refunded"
	ActionCancelled = "Order Cancelled"
)

var actions = []string{ActionCompleted, ActionUpdated, ActionRefunded, ActionCancelled}

// IsOrderAction reports whether action names one of the four order Events.
func IsOrderAction(action string) bool { return slices.Contains(actions, action) }

// The spec's property names this package reads.
const (
	propOrderID  = "order_id"
	propTotal    = "total"
	propCurrency = "currency"
	propRefundID = "refund_id"
)

// Parsed is what an order Event says, with its amount in the minor unit of its currency.
type Parsed struct {
	OrderID string
	// Currency is the currency the Event is in; "" when it named none and there is no default
	// to read it in (Deferred).
	Currency string
	// Deferred is an Event that names no currency while none is given as the default: its
	// amount is a valid number in no currency yet, so Total is not set.
	Deferred bool
	// Total is the amount of the Event; HasTotal is false for an Order Cancelled that sent none.
	Total    int64
	HasTotal bool
	// RefundID is the optional `refund_id` of an Order Refunded, "" when it sent none.
	RefundID string
}

// RefusalError is why an order Event was refused, worded for the one who sent it.
type RefusalError struct{ Reason string }

func (e *RefusalError) Error() string { return e.Reason }

func refuse(format string, args ...any) error {
	return &RefusalError{Reason: fmt.Sprintf(format, args...)}
}

// IsRefusal reports whether err is a RefusalError.
func IsRefusal(err error) bool {
	var r *RefusalError
	return errors.As(err, &r)
}

// Parse checks an order Event's properties and reads them, and only what Sphericon reads: the
// order_id, the total, the currency and a refund's refund_id. Every other property is kept in the
// Event and ignored here. fallback is the currency of an Event that names none (nil for none): the
// Reporting currency at ingest, the Order's own for a change to an existing Order. With neither
// the Event is Deferred: its total must be a number, and it is read in a currency later.
func Parse(action string, props map[string]any, fallback *string) (Parsed, error) {
	var p Parsed
	id, ok := props[propOrderID].(string)
	if !ok || id == "" {
		return p, refuse("%s: order_id is required and is text", action)
	}
	p.OrderID = id

	code, err := currencyOf(action, props, fallback)
	if err != nil {
		return p, err
	}
	p.Currency, p.Deferred = code, code == ""

	raw, has := props[propTotal]
	switch {
	case has && raw != nil && p.Deferred:
		if err := money.Check(raw); err != nil {
			return p, refuse("%s: total %s", action, err)
		}
		p.HasTotal = true
	case has && raw != nil:
		minor, err := money.Minor(raw, code)
		if err != nil {
			return p, refuse("%s: total %s", action, err)
		}
		p.Total, p.HasTotal = minor, true
	case action != ActionCancelled:
		return p, refuse("%s: total is required", action)
	}
	if action == ActionRefunded {
		refundID, err := refundIDOf(props)
		if err != nil {
			return p, err
		}
		p.RefundID = refundID
	}
	return p, nil
}

// Read is the properties Sphericon reads, for a held Event to keep.
func Read(props map[string]any) map[string]any {
	kept := make(map[string]any, 4)
	for _, name := range []string{propOrderID, propTotal, propCurrency, propRefundID} {
		if v, ok := props[name]; ok {
			kept[name] = v
		}
	}
	return kept
}

// refundIDOf reads the optional refund_id: text, or a whole number a shop keeps as one.
func refundIDOf(props map[string]any) (string, error) {
	switch id := props[propRefundID].(type) {
	case nil:
		return "", nil
	case string:
		if id == "" {
			return "", refuse("%s: refund_id is empty; leave it out to send none", ActionRefunded)
		}
		return id, nil
	case float64:
		if id == math.Trunc(id) && math.Abs(id) < 1<<53 {
			return strconv.FormatInt(int64(id), 10), nil
		}
	case json.Number:
		return id.String(), nil
	}
	return "", refuse("%s: refund_id must be text or a whole number", ActionRefunded)
}

func currencyOf(action string, props map[string]any, fallback *string) (string, error) {
	switch given := props[propCurrency].(type) {
	case nil:
		if fallback == nil {
			return "", nil
		}
		return *fallback, nil
	case string:
		if _, err := money.Exponent(given); err != nil {
			return "", refuse("%s: currency %q is not a known ISO 4217 code", action, given)
		}
		return given, nil
	default:
		return "", refuse("%s: currency must be an ISO 4217 code in capitals", action)
	}
}
