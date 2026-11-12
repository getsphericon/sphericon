package outboundmessages

import (
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// Observation is the anonymous report measurement carried to its owner after publication.
// Neither Contact identity nor the original event payload is needed to update a delivery row.
type Observation struct {
	MessageID  int64     `json:"messageId"`
	Action     string    `json:"action"`
	ObservedAt time.Time `json:"observedAt"`
}

// ObservationOf selects lasting report measurements, excluding machine engagement and SMS's
// final-state transition. It reads only the typed event, never mutable rows.
func ObservationOf(env events.Envelope, ev events.DomainEvent) (Observation, bool) {
	o := Observation{ObservedAt: env.OccurredAt}
	switch e := ev.(type) {
	case *events.EmailEngagement:
		if e.Machine || (e.Action != systemevent.EmailOpened && e.Action != systemevent.EmailClicked) {
			return o, false
		}
		o.MessageID, o.Action = e.OutboundMessageID, e.Action
	case *events.EmailDelivery:
		if e.Action != systemevent.EmailDelivered && e.Action != systemevent.EmailBounced && e.Action != systemevent.EmailComplained {
			return o, false
		}
		o.MessageID, o.Action = e.OutboundMessageID, e.Action
	case *events.SmsClicked:
		if e.Machine {
			return o, false
		}
		o.MessageID, o.Action = e.OutboundMessageID, systemevent.SmsClicked
	case *events.TelegramClicked:
		if e.Machine {
			return o, false
		}
		o.MessageID, o.Action = e.OutboundMessageID, systemevent.TelegramClicked
	default:
		return o, false
	}
	return o, o.MessageID != 0
}

// RecordObservation keeps the earliest observation regardless of delivery order or retries.
// An anonymous Outbound message is still a report record; this updates no personal field.
func RecordObservation(ctx context.Context, s *ent.Scoped, o Observation) error {
	switch o.Action {
	case systemevent.EmailOpened:
		_, err := StampOpened(ctx, s, o.MessageID, o.ObservedAt)
		return err
	case systemevent.EmailClicked, systemevent.SmsClicked, systemevent.TelegramClicked:
		_, err := StampClicked(ctx, s, o.MessageID, o.ObservedAt)
		return err
	case systemevent.EmailDelivered:
		return StampDelivery(ctx, s, o.MessageID, ResultDelivered, o.ObservedAt)
	case systemevent.EmailBounced:
		return StampDelivery(ctx, s, o.MessageID, ResultBounced, o.ObservedAt)
	case systemevent.EmailComplained:
		return StampDelivery(ctx, s, o.MessageID, ResultComplained, o.ObservedAt)
	default:
		return fmt.Errorf("unknown message observation %q", o.Action)
	}
}
