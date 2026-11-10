package webhookendpoints

import (
	"context"
	"slices"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// The rules of a delivery are the endpoint's (ADR 0107): which endpoints an event goes to, and
// whether a queued delivery still goes out and with which secret. The delivery job only sends.

// Subscribed returns the Workspace's enabled endpoints that receive the event named eventName: an
// endpoint with no filter receives every customer-facing Event, and an event that is not Projected
// (an Audit entry, ADR 0022) only when its filter names it.
func Subscribed(ctx context.Context, s *ent.Scoped, eventName string) ([]*ent.WebhookEndpoint, error) {
	endpoints, err := s.WebhookEndpoint().Query().Where(webhookendpoint.DisabledAtIsNil()).All(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(endpoints, func(e *ent.WebhookEndpoint) bool {
		return !receives(e.EventTypes, eventName)
	}), nil
}

// receives reports whether an endpoint with the given filter receives the event.
func receives(filter []string, name string) bool {
	if len(filter) == 0 {
		return !events.IsUnprojected(name)
	}
	return slices.Contains(filter, name)
}

// Delivery is where a queued delivery goes: the endpoint's URL and its opened signing secret.
type Delivery struct {
	URL, Secret string
}

// DeliveryTo is the Delivery of a queued delivery to e, and false when it no longer goes out: the
// endpoint was disabled since the delivery was queued.
func (m *Module) DeliveryTo(e *ent.WebhookEndpoint) (Delivery, bool, error) {
	if e.DisabledAt != nil {
		return Delivery{}, false, nil
	}
	secret, err := m.cipher.Decrypt(e.SecretEncrypted)
	if err != nil {
		return Delivery{}, false, err
	}
	return Delivery{URL: e.URL, Secret: string(secret)}, true, nil
}
