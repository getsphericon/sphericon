package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/publicid"

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
	WorkspaceID string          `json:"workspaceId"`
	Subject     string          `json:"subject"`
	ContactID   publicid.ID     `json:"contactId,omitempty"`
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
// on the semantic action (e.g. "page_view"), not the bus type. An event that is not Projected (an
// Audit entry) has no Event action: it is delivered under its bus type, only to endpoints that
// select it, and the Edition's dispatcher gates it by license (ADR 0022); the Staff identity in it
// is masked.
func WebhookDeliveryOf(env Envelope, workspacePublicID string) (WebhookDelivery, error) {
	ev, err := Decode(env)
	if err != nil {
		return WebhookDelivery{}, err
	}
	p, projected := ProjectionOf(ev)
	name := p.Action
	if !projected {
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
	if data, err = publicData(data, workspacePublicID); err != nil {
		return WebhookDelivery{}, err
	}
	// An event with a natural upstream id (a provider notification) is delivered under it, so
	// a receiver that sees it twice can drop the second delivery.
	id := env.SourceID()
	body, err := json.Marshal(webhookPayload{
		ID:          id,
		Type:        name,
		OccurredAt:  env.OccurredAt,
		WorkspaceID: workspacePublicID,
		Subject:     p.Subject,
		ContactID:   publicid.ID(p.ContactID),
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

// publicData is an event's data as a receiver sees it, with every key the APIs show by its public
// form (ADR 0142): workspaceId is the Workspace's public id, and a top-level key named …Id or …Ids
// whose value is a number (a primary key; an id a customer or a provider gave is a string) is
// sealed (publicid). Nested objects are the customer's own, a collected event's properties, and
// stay as they are.
func publicData(data json.RawMessage, workspacePublicID string) (json.RawMessage, error) {
	if len(data) == 0 {
		return data, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var fields map[string]any
	if err := dec.Decode(&fields); err != nil {
		return nil, fmt.Errorf("read event data: %w", err)
	}
	for key, value := range fields {
		switch {
		case key == "workspaceId":
			fields[key] = workspacePublicID
		case strings.HasSuffix(key, "Id"):
			if n, ok := value.(json.Number); ok {
				if fields[key], ok = sealed(n); !ok {
					return nil, fmt.Errorf("event data %s is no key: %s", key, n)
				}
			}
		case strings.HasSuffix(key, "Ids"):
			if list, ok := value.([]any); ok {
				for i, item := range list {
					if n, ok := item.(json.Number); ok {
						if list[i], ok = sealed(n); !ok {
							return nil, fmt.Errorf("event data %s holds no key: %s", key, n)
						}
					}
				}
			}
		}
	}
	return json.Marshal(fields)
}

// sealed is the public form of a key read as a JSON number.
func sealed(n json.Number) (string, bool) {
	key, err := n.Int64()
	if err != nil {
		return "", false
	}
	return publicid.Encode(key), true
}
