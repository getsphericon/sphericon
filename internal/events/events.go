// Package events is the internal domain-event system: a domain event is published
// once, inside the same transaction that commits the state change (transactional
// outbox), and fanned out to independent subscribers over watermill. See
// docs/design/domain-events.md.
//
// Two categories of events, never conflated:
//   - OUR events are a closed, typed union (ContactCreated, EmailEngagement, …).
//     Each Go type owns its projection via Project().
//   - A CUSTOMER's event (page_view, added_to_cart) is the user's domain — opaque
//     to us, stored as-is. Ingesting one is itself OUR typed event, CollectedEvent,
//     whose body carries the user's action + properties verbatim.
//
// On the wire the envelope carries the typed event as Data (json); subscribers
// decode it back into the concrete type via the name registry and call Project().
package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/internal/suppression"
)

// TopicDomainEvents is the single watermill topic all domain events flow through.
// watermill-sql maps it to "watermill_domain_events", which is the outbox table.
const TopicDomainEvents = "domain_events"

// Bus event names — the stable type discriminators (registry keys). These are
// OURS and finite. A customer's action name is NOT one of these; it lives inside
// CollectedEvent.Action.
const (
	NameContactCreated     = "contact.created"
	NameContactErased      = "contact.erased"
	NameEmailSent          = "email.sent"
	NameEmailDelivered     = "email.delivered"
	NameEmailDelayed       = "email.delayed"
	NameEmailDropped       = "email.dropped"
	NameEmailOpened        = "email.opened"
	NameEmailClicked       = "email.clicked"
	NameEmailUnsubscribed  = "email.unsubscribed"
	NameEmailBounced       = suppression.EventBounced
	NameEmailComplained    = suppression.EventComplained
	NameMarketingConfirmed = "marketing.confirmed"
	NameCollected          = "event.collected"
)

// reservedPrefixes are the namespaces of the actions only the system records: delivery
// (`email.*`), consent (`marketing.*`) and the Contact lifecycle (`contact.*`). They are
// evidence (ADR 0013, ADR 0019), so a customer's own Event never carries one.
var reservedPrefixes = []string{"email.", "marketing.", "contact."}

// ReservedAction reports whether action belongs to the system and is refused from a
// customer's Event.
func ReservedAction(action string) bool {
	a := strings.ToLower(strings.TrimSpace(action))
	return slices.ContainsFunc(reservedPrefixes, func(prefix string) bool { return strings.HasPrefix(a, prefix) })
}

// The surfaces a customer's client event id arrives on; each has its own id space.
const (
	SourceCollect = "collect"
	SourceAPI     = "api"
)

// DedupKey is the Event's source_id for a client event id: source_id is unique across
// Workspaces and sources, the client's id only within one of each.
func DedupKey(source string, workspaceID int64, clientEventID string) string {
	return fmt.Sprintf("%s:%d:%s", source, workspaceID, clientEventID)
}

// RejectionError is why a customer's Event was refused. Its message is worded for the caller.
type RejectionError struct{ Reason string }

func (r *RejectionError) Error() string { return r.Reason }

// AsRejection reports whether err is a RejectionError, and which.
func AsRejection(err error) (*RejectionError, bool) {
	var r *RejectionError
	ok := errors.As(err, &r)
	return r, ok
}

// CheckAction refuses an action that is empty or reserved for the system.
func CheckAction(action string) error {
	switch {
	case strings.TrimSpace(action) == "":
		return &RejectionError{"subject id and action are required"}
	case ReservedAction(action):
		return &RejectionError{fmt.Sprintf("action %q is reserved for the system", action)}
	}
	return nil
}

// Bounce kinds carried by EmailDelivery; what each means for Suppression is the
// suppression module's rule.
const (
	BounceKindHard = suppression.BounceKindHard
	BounceKindSoft = suppression.BounceKindSoft
)

// Envelope is the generic bus message. It carries transport metadata plus the
// typed event as Data; subscribers decode Data via the registry.
type Envelope struct {
	ID          string          `json:"id"`          // ULID; the watermill message id
	Name        string          `json:"name"`        // bus event type (registry key)
	Version     int             `json:"version"`     // schema version of Data
	WorkspaceID int64           `json:"workspaceId"` // every event is workspace-scoped
	OccurredAt  time.Time       `json:"occurredAt"`
	Data        json.RawMessage `json:"data"` // the typed event, marshaled
	// DedupKey is the persist idempotency key. Empty ⇒ use ID. An event with a
	// natural upstream id (a provider notification) sets it so at-least-once
	// redelivery dedupes, while ID stays a short ULID for the message-id column.
	DedupKey string `json:"dedupKey,omitempty"`
	// RecordedAt is when the event was published, as opposed to OccurredAt, which a client may
	// backdate. Rules act only on events recorded after they were activated (ADR 0054).
	RecordedAt time.Time `json:"recordedAt"`
	// CausedBy names what published the event when it was not a customer or a provider: an
	// Action of a Rule stamps it on any event it publishes, and the Rule consumer skips an event
	// that carries it, so Rules never chain (ADR 0054). No Action publishes an event today, so
	// nothing stamps it yet; the consumer honours it already.
	CausedBy string `json:"causedBy,omitempty"`
}

// Projection is the durable Event-row an event maps to. Each event type computes
// its own via Project(), so the projection logic lives with the type rather than
// being flattened by producers.
type Projection struct {
	Subject    string         // → events.subject_id snapshot; "" ⇒ anonymous
	Action     string         // → events.action; also the automation trigger key
	Email      string         // "" ⇒ unset
	Phone      string         // "" ⇒ unset
	Properties map[string]any // → events.properties (opaque for customer events)
	OccurredAt time.Time      // zero ⇒ publish time
	// ContactID is the authoritative Contact link (→ events.contact_id) AND the
	// automations enroll target; 0 ⇒ none (anonymous, pre-Identify).
	ContactID int64
	// VisitorID is the anonymous device (→ events.visitor_id), kept so an anonymous
	// event can be stitched onto a Contact at Identify. "" ⇒ unset.
	VisitorID string
	// AutomationID is the Automation whose email the Event came from, 0 for any other Event. An
	// Automation never starts from an Event of its own email (ADR 0053).
	AutomationID int64
	// Machine marks an open or click made by software (ADR 0053). It is recorded, but starts no
	// Automation.
	Machine bool
}

// DomainEvent is a typed, producer-side domain event. Implementations are
// registered in registry and must use pointer receivers so Decode can unmarshal.
type DomainEvent interface {
	EventName() string   // bus type / registry key
	EventVersion() int   // schema version
	Workspace() int64    // owning workspace
	Project() Projection // how this event maps to the Event row + trigger
}

// identifiable is an optional DomainEvent capability: an event with a stable
// upstream id (a provider notification, an external message) implements it so the
// publisher records that id as the envelope's DedupKey, making at-least-once
// redelivery idempotent at the persist layer (which dedupes on source_id). The
// id does NOT become the message id, which stays a short ULID.
type identifiable interface {
	EventID() string
}

// backfilled is an optional DomainEvent capability: an event that is history, recorded but
// never reacted to, so the webhooks and automations consumers skip it.
type backfilled interface {
	Backfilled() bool
}

// IsBackfill reports whether ev is a Backfill: recorded without triggering automations or webhooks.
func IsBackfill(ev DomainEvent) bool {
	b, ok := ev.(backfilled)
	return ok && b.Backfilled()
}

// registry maps a bus event name to a constructor for its concrete type, so a
// serialized envelope can be decoded back into the right Go type. All variants
// are ours and finite — there is no catch-all.
var registry = map[string]func() DomainEvent{
	NameContactCreated:        func() DomainEvent { return &ContactCreated{} },
	NameContactErased:         func() DomainEvent { return &ContactErased{} },
	NameEmailSent:             func() DomainEvent { return &EmailEngagement{} },
	NameEmailOpened:           func() DomainEvent { return &EmailEngagement{} },
	NameEmailClicked:          func() DomainEvent { return &EmailEngagement{} },
	NameEmailUnsubscribed:     func() DomainEvent { return &EmailEngagement{} },
	NameEmailDelivered:        func() DomainEvent { return &EmailDelivery{} },
	NameEmailDelayed:          func() DomainEvent { return &EmailDelivery{} },
	NameEmailDropped:          func() DomainEvent { return &EmailDelivery{} },
	NameEmailBounced:          func() DomainEvent { return &EmailDelivery{} },
	NameEmailComplained:       func() DomainEvent { return &EmailDelivery{} },
	NameMarketingConfirmed:    func() DomainEvent { return &MarketingConfirmed{} },
	NameCollected:             func() DomainEvent { return &CollectedEvent{} },
	NameAuditEntry:            func() DomainEvent { return &AuditEntry{} },
	NameTrackingDomainChanged: func() DomainEvent { return &TrackingDomainChanged{} },
}

// Decode reconstructs the typed event from an envelope.
func Decode(env Envelope) (DomainEvent, error) {
	newEvent, ok := registry[env.Name]
	if !ok {
		return nil, fmt.Errorf("events: unknown event %q", env.Name)
	}
	ev := newEvent()
	if err := json.Unmarshal(env.Data, ev); err != nil {
		return nil, fmt.Errorf("events: decode %q: %w", env.Name, err)
	}
	return ev, nil
}

// ContactCreated is emitted when a contact is created in a workspace.
type ContactCreated struct {
	WorkspaceID int64  `json:"workspaceId"`
	ContactID   int64  `json:"contactId"`
	Email       string `json:"email"`
}

func (*ContactCreated) EventName() string  { return NameContactCreated }
func (*ContactCreated) EventVersion() int  { return 1 }
func (e *ContactCreated) Workspace() int64 { return e.WorkspaceID }
func (e *ContactCreated) Project() Projection {
	return Projection{Subject: e.Email, Action: NameContactCreated, Email: e.Email, ContactID: e.ContactID}
}

// ContactErased is emitted once per Erasure (ADR 0021): the PII-free accountability
// record, and the signal the customer's webhook uses to erase downstream copies.
// SubjectID is the Contact's own subject_id, or its sphericon id when it had none. It
// exists only in this message (so the webhook payload carries it): Project() leaves
// it, and any Contact reference or address, off the stored Event. The keys below it
// are for the Erasures Export stream and stay off the stored Event too.
type ContactErased struct {
	WorkspaceID    int64  `json:"workspaceId"`
	SubjectID      string `json:"subjectId"`
	IdentifierKind string `json:"identifierKind"` // what the requester identified the person by
	RequesterKind  string `json:"requesterKind"`  // user|api_token
	RequesterID    int64  `json:"requesterId"`

	// The keys Erasure matched on (ADR 0045), for the Erasures Export stream: the erased
	// Contact's id (the row is gone, so the id points at nothing), the anonymous devices
	// erased with it, and SHA-256 (hex) of the lowercased address and of the phone, computed
	// before publishing so no address or phone reaches the outbox. Empty when there was none.
	ContactID  int64    `json:"contactId,omitempty"`
	VisitorIDs []string `json:"visitorIds,omitempty"`
	EmailHash  string   `json:"emailHash,omitempty"`
	PhoneHash  string   `json:"phoneHash,omitempty"`
}

func (*ContactErased) EventName() string  { return NameContactErased }
func (*ContactErased) EventVersion() int  { return 1 }
func (e *ContactErased) Workspace() int64 { return e.WorkspaceID }
func (e *ContactErased) Project() Projection {
	return Projection{Action: NameContactErased, Properties: map[string]any{
		"identifierKind": e.IdentifierKind,
		"requesterKind":  e.RequesterKind,
		"requesterId":    e.RequesterID,
	}}
}

// EmailEngagement is emitted when one of our emails is sent to a recipient, or
// when the recipient opens, clicks, or unsubscribes from a broadcast email.
// Action is the specific name; URL is set for clicks only. DedupID, when set,
// makes the persist projection idempotent under at-least-once redelivery — the
// send path sets it (a deterministic per-recipient/per-step key) so a job retry
// can't write a second email.sent row; open/click/unsubscribe leave it empty and
// fall back to the envelope ULID.
type EmailEngagement struct {
	Action      string `json:"action"` // email.sent|email.opened|email.clicked|email.unsubscribed
	WorkspaceID int64  `json:"workspaceId"`
	ContactID   int64  `json:"contactId"`
	Email       string `json:"email"`
	BroadcastID int64  `json:"broadcastId"`
	URL         string `json:"url,omitempty"`
	DedupID     string `json:"dedupId,omitempty"`
	// Machine is set on an open or click made by software (ADR 0053): it stays in the Event log
	// but counts toward no figure, sets no rollup and starts no Automation.
	Machine bool `json:"machine,omitempty"`

	// The envelope an Outbound send stamps on email.sent (ADR 0015, ADR 0011):
	// which surface sent it, under which Sending source and Sending domain, and the
	// ids that tie the Event back to the Outbound message and the provider's copy.
	Surface           string `json:"surface,omitempty"`
	SendingSource     string `json:"sendingSource,omitempty"`
	SendingDomain     string `json:"sendingDomain,omitempty"`
	AutomationID      int64  `json:"automationId,omitempty"`
	OutboundMessageID int64  `json:"outboundMessageId,omitempty"`
	ProviderMessageID string `json:"providerMessageId,omitempty"`
	// Captured is set when the send went through a sender that delivers nowhere outside (a
	// Test inbox): Activity says "captured, not delivered".
	Captured bool `json:"captured,omitempty"`
}

func (e *EmailEngagement) EventName() string { return e.Action }
func (*EmailEngagement) EventVersion() int   { return 1 }
func (e *EmailEngagement) Workspace() int64  { return e.WorkspaceID }
func (e *EmailEngagement) EventID() string   { return e.DedupID }
func (e *EmailEngagement) Project() Projection {
	props := map[string]any{}
	// Automation sends carry no broadcast, so omit broadcastId rather than write 0 —
	// otherwise a "broadcastId = 0" segment filter would select every automation send.
	if e.BroadcastID != 0 {
		props["broadcastId"] = e.BroadcastID
	}
	if e.URL != "" {
		props["url"] = e.URL
	}
	for k, v := range map[string]string{
		"surface":           e.Surface,
		"sendingSource":     e.SendingSource,
		"sendingDomain":     e.SendingDomain,
		"providerMessageId": e.ProviderMessageID,
	} {
		if v != "" {
			props[k] = v
		}
	}
	if e.AutomationID != 0 {
		props["automationId"] = e.AutomationID
	}
	if e.OutboundMessageID != 0 {
		props["outboundMessageId"] = e.OutboundMessageID
	}
	if e.Captured {
		props[CapturedProperty] = true
	}
	if e.Machine {
		props[MachineProperty] = true
	}
	return Projection{Subject: e.Email, Action: e.Action, Email: e.Email, Properties: props, ContactID: e.ContactID, AutomationID: e.AutomationID, Machine: e.Machine}
}

// EmailDelivery is emitted when a provider reports what became of one of our sends: a
// Delivery notice (ADR 0053) ingested from a provider callback, e.g. SES via SNS. Action is
// the specific name. BounceKind distinguishes a hard bounce from a soft one and is empty for
// the other kinds; only hard bounces and complaints suppress the address. ContactID is 0 when
// the address has no contact in the workspace; OutboundMessageID is 0 when the notice named no
// message or none matched, and the Event then stays at the level of the address.
type EmailDelivery struct {
	Action      string `json:"action"` // email.delivered|email.delayed|email.dropped|email.bounced|email.complained
	WorkspaceID int64  `json:"workspaceId"`
	ContactID   int64  `json:"contactId,omitempty"`
	Email       string `json:"email"`
	BounceKind  string `json:"bounceKind,omitempty"` // hard|soft (bounces only)
	Provider    string `json:"provider,omitempty"`   // ses, ...
	// SendingDomain is the domain the failed message was sent from (ADR 0011), so the
	// complaint/bounce rate can be grained per Sending domain.
	SendingDomain string `json:"sendingDomain,omitempty"`
	// Reason is the provider's short explanation of a bounce, delay or drop, at most
	// 500 characters.
	Reason string `json:"reason,omitempty"`
	// ProviderMessageID is the id the notice named, kept even when no Outbound message
	// matched it.
	ProviderMessageID string `json:"providerMessageId,omitempty"`
	// The Outbound message the notice was linked to, and where that message came from.
	OutboundMessageID int64  `json:"outboundMessageId,omitempty"`
	Surface           string `json:"surface,omitempty"`
	SendingSource     string `json:"sendingSource,omitempty"`
	BroadcastID       int64  `json:"broadcastId,omitempty"`
	AutomationID      int64  `json:"automationId,omitempty"`
	// DedupID is the stable upstream id (e.g. SNS messageId + recipient) used as
	// the envelope id so a redelivered provider notification dedupes at persist.
	DedupID string `json:"dedupId,omitempty"`
	// Captured is set when the message went through a sender that delivers nowhere outside.
	Captured bool `json:"captured,omitempty"`
}

func (e *EmailDelivery) EventName() string { return e.Action }
func (*EmailDelivery) EventVersion() int   { return 1 }
func (e *EmailDelivery) Workspace() int64  { return e.WorkspaceID }
func (e *EmailDelivery) EventID() string   { return e.DedupID }
func (e *EmailDelivery) Project() Projection {
	props := map[string]any{}
	for k, v := range map[string]string{
		"bounceKind":        e.BounceKind,
		"provider":          e.Provider,
		"sendingDomain":     e.SendingDomain,
		"reason":            e.Reason,
		"providerMessageId": e.ProviderMessageID,
		"surface":           e.Surface,
		"sendingSource":     e.SendingSource,
	} {
		if v != "" {
			props[k] = v
		}
	}
	for k, v := range map[string]int64{
		"outboundMessageId": e.OutboundMessageID,
		"broadcastId":       e.BroadcastID,
		"automationId":      e.AutomationID,
	} {
		if v != 0 {
			props[k] = v
		}
	}
	if e.Captured {
		props[CapturedProperty] = true
	}
	return Projection{Subject: e.Email, Action: e.Action, Email: e.Email, Properties: props, ContactID: e.ContactID}
}

// MarketingConfirmed is emitted when a destination confirms its subscription
// (double opt-in, ADR 0013). It is the immutable source of truth for the
// confirmation — the derived Confirmation row is the read-model — and carries the
// GDPR Art. 7 proof: when, how (provenance), and the confirming IP. Publishing it
// also lets a welcome automation trigger on "marketing.confirmed".
type MarketingConfirmed struct {
	WorkspaceID int64  `json:"workspaceId"`
	ContactID   int64  `json:"contactId,omitempty"`
	Email       string `json:"email"`
	Provenance  string `json:"provenance"`   // double_opt_in|grandfathered|imported
	IP          string `json:"ip,omitempty"` // confirming client IP (double_opt_in only)
}

func (*MarketingConfirmed) EventName() string  { return NameMarketingConfirmed }
func (*MarketingConfirmed) EventVersion() int  { return 1 }
func (e *MarketingConfirmed) Workspace() int64 { return e.WorkspaceID }
func (e *MarketingConfirmed) Project() Projection {
	props := map[string]any{}
	if e.Provenance != "" {
		props["provenance"] = e.Provenance
	}
	if e.IP != "" {
		props["ip"] = e.IP
	}
	return Projection{Subject: e.Email, Action: NameMarketingConfirmed, Email: e.Email, Properties: props, ContactID: e.ContactID}
}

// CollectedEvent wraps a customer's own event ingested via the collect or external
// API. Its body is the user's domain, stored as-is: Action and Properties are
// opaque to us. Identity is resolved at ingest: ContactID is the Contact this device
// resolves to (0 ⇒ anonymous, pre-Identify), VisitorID the anonymous device — kept
// so the event can be stitched onto a Contact when Identify later arrives. Its Action
// is also the automation trigger key (a user can trigger on "page_view").
type CollectedEvent struct {
	WorkspaceID int64          `json:"workspaceId"`
	ContactID   int64          `json:"contactId,omitempty"` // 0 ⇒ anonymous
	VisitorID   string         `json:"visitorId,omitempty"`
	SubjectID   string         `json:"subjectId,omitempty"`
	Action      string         `json:"action"`
	Email       string         `json:"email,omitempty"`
	Phone       string         `json:"phone,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
	OccurredAt  time.Time      `json:"occurredAt,omitzero"`
	// DedupID is the persist idempotency key of an event the client gave a clientEventId
	// (DedupKey: the source, the Workspace and that id). Empty for an event without one.
	DedupID string `json:"dedupId,omitempty"`
	// Backfill marks an event loaded from history: it is recorded, but no Automation enrolls
	// on it and no webhook endpoint receives it.
	Backfill bool `json:"backfill,omitempty"`
}

func (*CollectedEvent) EventName() string  { return NameCollected }
func (*CollectedEvent) EventVersion() int  { return 1 }
func (e *CollectedEvent) Workspace() int64 { return e.WorkspaceID }
func (e *CollectedEvent) EventID() string  { return e.DedupID }
func (e *CollectedEvent) Backfilled() bool { return e.Backfill }
func (e *CollectedEvent) Project() Projection {
	return Projection{
		Subject:    e.SubjectID,
		Action:     e.Action,
		Email:      e.Email,
		Phone:      e.Phone,
		Properties: e.Properties,
		OccurredAt: e.OccurredAt,
		ContactID:  e.ContactID,
		VisitorID:  e.VisitorID,
	}
}
