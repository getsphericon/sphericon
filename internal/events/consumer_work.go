package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
)

// WebhookDispatcher fans an event out to the workspace's matching webhook
// endpoints. The webhooks consumer delegates to it, keeping durable delivery
// (per-endpoint jobs, retries) in river.
type WebhookDispatcher interface {
	Dispatch(ctx context.Context, s *ent.Scoped, eventName, deliveryID string, body []byte) error
}

// webhookPayload is the public JSON delivered to endpoints (and signed).
type webhookPayload struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	OccurredAt  time.Time       `json:"occurredAt"`
	WorkspaceID int64           `json:"workspaceId"`
	Subject     string          `json:"subject"`
	ContactID   int64           `json:"contactId,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
}

// WebhookDelivery is what a Webhook endpoint receives for an event (ADR 0039): the name an
// endpoint selects it by, the id a receiver drops a repeated delivery by, and the public body,
// which is signed.
type WebhookDelivery struct {
	Name string
	ID   string
	Body []byte
}

// WebhookDeliveryOf serializes the event of env into its public webhook body. Endpoints select
// on the semantic action (e.g. "page_view"), not the bus type. An unprojected event (an Audit
// entry) has no Event action: it is delivered under its bus type, only to endpoints that select
// it, and the Edition's dispatcher gates it by license (ADR 0022); the Staff identity in it is
// masked.
func WebhookDeliveryOf(env Envelope) (WebhookDelivery, error) {
	ev, err := Decode(env)
	if err != nil {
		return WebhookDelivery{}, err
	}
	p := ev.Project()
	name := p.Action
	if _, unprojected := ev.(Unprojected); unprojected {
		name = env.Name
	}
	data := env.Data
	if entry, ok := ev.(*AuditEntry); ok {
		// The staff identity never reaches a customer's endpoint (ADR 0022).
		entry.MaskStaff()
		if data, err = json.Marshal(entry); err != nil {
			return WebhookDelivery{}, fmt.Errorf("marshal audit entry: %w", err)
		}
	}
	// An event with a natural upstream id (a provider notification) is delivered under it, so
	// a receiver that sees it twice can drop the second delivery.
	id := env.SourceID()
	body, err := json.Marshal(webhookPayload{
		ID:          id,
		Type:        name,
		OccurredAt:  env.OccurredAt,
		WorkspaceID: env.WorkspaceID,
		Subject:     p.Subject,
		ContactID:   p.ContactID,
		Data:        data,
	})
	if err != nil {
		return WebhookDelivery{}, fmt.Errorf("marshal webhook payload: %w", err)
	}
	return WebhookDelivery{Name: name, ID: id, Body: body}, nil
}

// PropertiesBound is the most JSON an Event's properties may weigh to be shown to a Rule or copied
// onto an Enrollment (#752): the per-event limit ingest applies by default, so ingest already refuses
// larger customer events. A larger one reaching a Rule or an Enrollment anyway has no properties,
// for the Condition and for the Firing's Actions alike, and for every step of the Enrollment, so a
// Rule never decides on a value its Actions could not template, and neither a job row nor an
// Enrollment's row grows without limit.
const PropertiesBound = 32 << 10

// BoundedProperties is the view of an Event's properties that a Rule evaluates, a Firing keeps and
// an Enrollment copies: the properties themselves, or nil when they weigh more than
// PropertiesBound. Every path that shows properties to a Rule (the consumer, Evaluate, a dry run)
// or copies them onto an Enrollment goes through it.
func BoundedProperties(props map[string]any) map[string]any {
	if len(props) == 0 {
		return props
	}
	if encoded, err := json.Marshal(props); err != nil || len(encoded) > PropertiesBound {
		return nil
	}
	return props
}
