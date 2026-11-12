package jobs

import (
	"context"

	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// RecordSendOutcome counts a provider's bounce or complaint on the send-outcome counter. The bus
// runs it for every Event newly stored, once the Event's transaction committed
// (events.Bus.OnStored), so neither a repeated notice nor a rolled-back one counts. It is an event
// subscriber, a driver (review rule 5), which the wiring registers on the bus.
func RecordSendOutcome(ctx context.Context, ev events.DomainEvent) {
	f, ok := ev.(*events.EmailDelivery)
	if !ok || f.Provider == "" {
		return
	}
	var status messaging.SendStatus
	switch f.Action {
	case systemevent.EmailBounced:
		status = messaging.SendBounce
	case systemevent.EmailComplained:
		status = messaging.SendComplaint
	default:
		return
	}
	// Provider is the Preset's name (ADR 0037), or "ses" for the SES adapter.
	messaging.RecordSendOutcome(ctx, providers.Provider(f.Provider), status)
}
