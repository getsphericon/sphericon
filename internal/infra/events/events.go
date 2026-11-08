// Package events is the internal domain-event system: a domain event is published
// once, inside the same transaction that commits the state change, which stores its
// Event row and inserts one River job for every consumer that wants it (ADR 0101). See
// docs/design/domain-events.md.
//
// Two categories of events, never conflated:
//   - OUR events are a closed, typed union (ContactCreated, EmailEngagement, …).
//     Each Go type owns its projection via Project().
//   - A CUSTOMER's event (page_view, added_to_cart) is the user's domain — opaque
//     to us, stored as-is. Ingesting one is itself OUR typed event, CollectedEvent,
//     whose body carries the user's action + properties verbatim.
//
// An event belongs to a Workspace or to the Instance (a User's sign-up, a password reset): only a
// Workspace's event can be an Event of the Workspace, with a row of its own (Projected). An event
// job carries the envelope, with the typed event as Data (json); a consumer decodes it back into
// the concrete type via the name registry.
package events

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

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
	NameEmailBounced       = "email.bounced"
	NameEmailComplained    = "email.complained"
	NameMarketingConfirmed = "marketing.confirmed"
	NameCollected          = "event.collected"

	// NameContactTagged and the two below are what happened to a Contact's Tags and Custom
	// fields, recorded only on a real change.
	NameContactTagged       = "contact.tagged"
	NameContactUntagged     = "contact.untagged"
	NameContactFieldChanged = "contact.field_changed"

	// NameAutomationEnrolled and the three below are an Enrollment's beginning and its ends, all
	// but `failed`: an operational failure, not something the Contact did.
	NameAutomationEnrolled    = "automation.enrolled"
	NameAutomationCompleted   = "automation.completed"
	NameAutomationGoalReached = "automation.goal_reached"
	NameAutomationExited      = "automation.exited"

	// NameTelegramBound and the one below are a Telegram chat Bound to a Contact and a Binding
	// moved away from one (ADR 0076).
	NameTelegramBound   = "telegram.bound"
	NameTelegramUnbound = "telegram.unbound"
	// NameTelegramReceived is a message a bound Contact sent a bot (ADR 0076).
	NameTelegramReceived = "telegram.received"
	// NameTelegramSent is a message a bot's Telegram accepted for a Contact. Telegram reports
	// nothing after that, so no delivered or read Event follows.
	NameTelegramSent = "telegram.sent"
	// NameTelegramClicked is a click on a short tracked link of a Telegram message (ADR 0083).
	NameTelegramClicked = "telegram.clicked"
	// NameTelegramUnsubscribed is a Contact stopping all of the Workspace's Telegram marketing in
	// Telegram: a STOP keyword, the unsubscribe button, or blocking the bot (ADR 0077).
	NameTelegramUnsubscribed = "telegram.unsubscribed"

	// NameSmsReceived is a text a Contact sent from its phone to an SMS Integration's sender
	// (ADR 0076).
	NameSmsReceived = "sms.received"
	// NameSmsSent is a text the SMS provider accepted for a Contact (ADR 0079): handed to the
	// provider, not delivered; a delivery report says that.
	NameSmsSent = "sms.sent"
	// NameSmsDelivered and the one below are the SMS provider's final report of a text it accepted
	// (ADR 0079): it reached the handset, or it did not, with the provider's error code.
	NameSmsDelivered   = "sms.delivered"
	NameSmsUndelivered = "sms.undelivered"
	// NameSmsConfirmed is a phone number's SMS Confirmation (ADR 0077), the proof of its consent;
	// marketing.confirmed is email's alone.
	NameSmsConfirmed = "sms.confirmed"
	// NameSmsClicked is a click on a short tracked link of an SMS (ADR 0083).
	NameSmsClicked = "sms.clicked"
	// NameSmsUnsubscribed is a Contact's number opting out of all of the Workspace's SMS marketing
	// (ADR 0077): a STOP keyword, the unsubscribe link, a send the provider refused because the
	// number had stopped, or by hand. Only a new opt-out records it, never a repeated stop (ADR 0114).
	NameSmsUnsubscribed = "sms.unsubscribed"
)

// reservedPrefixes are the namespaces of the actions only the system records: delivery
// (`email.*`), consent (`marketing.*`), the Contact's lifecycle, Tags and Custom fields
// (`contact.*`), its Enrollments (`automation.*`), its Telegram Bindings (`telegram.*`) and its SMS
// (`sms.*`). Some are evidence (ADR 0013, ADR 0019), and
// an Automation, a Rule or a Segment that names one must be sure the system recorded it, so a
// customer's own Event never carries one.
var reservedPrefixes = []string{"email.", "marketing.", "contact.", "automation.", "telegram.", "sms."}

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

// DedupKey namespaces a client event id by its intake source and Workspace for the public
// Event source id. The bus independently scopes consumption receipts to the Workspace and fact.
func DedupKey(source string, workspaceID int64, clientEventID string) string {
	return fmt.Sprintf("%s:%d:%s", source, workspaceID, clientEventID)
}

// ErrRejected is a customer's Event the platform refuses; Reject makes it With the reason, worded
// for the caller.
var ErrRejected = validation.Refuse("events: event rejected", i18n.ErrorsEventRejected)

// Reject is the refusal of a customer's Event for reason.
func Reject(reason string) error { return ErrRejected.With(reason) }

// CheckAction refuses an action that is empty or reserved for the system.
func CheckAction(action string) error {
	switch {
	case strings.TrimSpace(action) == "":
		return Reject("subject id and action are required")
	case ReservedAction(action):
		return Reject(fmt.Sprintf("action %q is reserved for the system", action))
	}
	return nil
}

// Bounce kinds carried by EmailDelivery. Only a hard bounce suppresses; a soft one, such as a
// full mailbox, is temporary. What each means for Suppression is the suppression module's rule.
const (
	BounceKindHard = "hard"
	BounceKindSoft = "soft"
)

// Envelope is the generic bus message. It carries transport metadata plus the
// typed event as Data; subscribers decode Data via the registry.
type Envelope struct {
	ID          string          `json:"id"`                    // ULID; the event's own id
	Name        string          `json:"name"`                  // bus event type (registry key)
	Version     int             `json:"version"`               // schema version of Data
	WorkspaceID int64           `json:"workspaceId,omitempty"` // 0: an event of the Instance, no Workspace's
	OccurredAt  time.Time       `json:"occurredAt"`
	Data        json.RawMessage `json:"data"` // the typed event, marshaled
	// DedupKey is the raw upstream identity. SourceID namespaces it by event name. An event with a
	// natural upstream id (a provider notification) sets it so a redelivery is a
	// repeat Publish drops, while ID stays the event's own ULID.
	DedupKey string `json:"dedupKey,omitempty"`
	// RecordedAt is when the event was published, as opposed to OccurredAt, which a client may
	// backdate. Rules act only on events recorded after they were activated (ADR 0054).
	RecordedAt time.Time `json:"recordedAt"`
	// CausedBy names what published the event when it was not a customer or a provider: an
	// Action of a Rule stamps it on any event it publishes, and the Rule consumer skips an event
	// that carries it, so Rules never chain (ADR 0054). The Action's work runs in a context
	// CausedBy made, which the publisher stamps it from.
	CausedBy string `json:"causedBy,omitempty"`
	// CorrelationID is shared by every event of one chain: what a request, or the handling of
	// an event, published (correlation.go). CausationID is the id of the event whose handling
	// published this one, empty at the start of a chain.
	CorrelationID string `json:"correlationId,omitempty"`
	CausationID   string `json:"causationId,omitempty"`
}

// SourceID is the id the Event's row is stored under (events.source_id): the event name and
// natural upstream identity when present, else the envelope ULID. A Firing is unique on it, and a
// trigger job carries it so an Enrollment can copy the stored Event's properties (#752).
func (e Envelope) SourceID() string {
	if e.DedupKey != "" {
		return e.Name + ":" + e.DedupKey
	}
	return e.ID
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
	// AutomationID is the Automation the Event came from (→ events.automation_id): the one whose
	// email it is about, or whose Enrollment of the Contact began or ended; 0 for any other Event.
	// An Automation never starts from an Event of its own (ADR 0053).
	AutomationID int64
	// BroadcastID is the Broadcast whose email the Event came from (→ events.broadcast_id), 0 for
	// any other Event.
	BroadcastID int64
	// URL is the link of a click (→ events.url), "" for any other Event.
	URL string
	// Machine marks an open or click made by software (ADR 0053, → events.machine). It is
	// recorded, but starts no Automation.
	Machine bool
	// Captured marks an Event of a send through a sender that delivers nowhere outside
	// (→ events.captured).
	Captured bool
}

// DomainEvent is a typed, producer-side domain event. Implementations are
// registered in registry and must use pointer receivers so Decode can unmarshal. An event is
// the Instance's (a User's sign-up, a password reset: no Workspace holds it) unless it is a
// WorkspaceEvent, and it has an Event row only when it is Projected.
type DomainEvent interface {
	EventName() string // bus type / registry key
	EventVersion() int // schema version
}

// WorkspaceEvent is a DomainEvent that belongs to a Workspace: its envelope names the Workspace,
// and a Workspace's deletion and a Contact's Erasure find its jobs by it.
type WorkspaceEvent interface {
	DomainEvent
	Workspace() int64 // owning workspace
}

// Projected is a WorkspaceEvent that is an Event of its Workspace (a data-plane fact): Publish
// writes its row from the projection, and the automations, the Rules and the webhooks react to
// it. Any other event (an Audit entry, a notice-worthy change of the Workspace's configuration,
// an event of the Instance) has no row and is seen only by the consumers that ask for it.
type Projected interface {
	WorkspaceEvent
	Project() Projection // how this event maps to the Event row + trigger
}

// WorkspaceOf is the Workspace ev belongs to, 0 for an event of the Instance.
func WorkspaceOf(ev DomainEvent) int64 {
	if w, ok := ev.(WorkspaceEvent); ok {
		return w.Workspace()
	}
	return 0
}

// ProjectionOf is ev's projection onto its Event row; ok is false for an event that has none.
func ProjectionOf(ev DomainEvent) (p Projection, ok bool) {
	if pe, ok := ev.(Projected); ok {
		return pe.Project(), true
	}
	return Projection{}, false
}

// IsUnprojected reports whether name is the bus type of events that are not Projected (an Audit
// entry): a name that is no bus type (a customer's action, a System event's action) is not.
func IsUnprojected(name string) bool {
	newEvent, ok := registry[name]
	if !ok {
		return false
	}
	_, projected := newEvent().(Projected)
	return !projected
}

// identifiable is an optional DomainEvent capability: an event with a stable
// upstream id (a provider notification, an external message) implements it so the
// publisher records that id as the envelope's DedupKey and claims it before reactions, including
// for a fact with no Event row. A Projected event namespaces it by name for source_id. The envelope id
// stays a fresh ULID, independent of that upstream identity.
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
	NameSmsStopObserved:           func() DomainEvent { return &SmsStopObserved{} },
	NameSmsStartObserved:          func() DomainEvent { return &SmsStartObserved{} },
	NameSmsDestinationUnreachable: func() DomainEvent { return &SmsDestinationUnreachable{} },
	NameSmsOptOutRecorded:         func() DomainEvent { return &SmsOptOutRecorded{} },
	NameSmsSuppressionRecorded:    func() DomainEvent { return &SmsSuppressionRecorded{} },
	NameMessageUnsubscribed:       func() DomainEvent { return &MessageUnsubscribed{} },
	NameContactCreated:            func() DomainEvent { return &ContactCreated{} },
	NameContactErased:             func() DomainEvent { return &ContactErased{} },
	NameEmailSent:                 func() DomainEvent { return &EmailEngagement{} },
	NameEmailOpened:               func() DomainEvent { return &EmailEngagement{} },
	NameEmailClicked:              func() DomainEvent { return &EmailEngagement{} },
	NameEmailUnsubscribed:         func() DomainEvent { return &EmailEngagement{} },
	NameEmailDelivered:            func() DomainEvent { return &EmailDelivery{} },
	NameEmailDelayed:              func() DomainEvent { return &EmailDelivery{} },
	NameEmailDropped:              func() DomainEvent { return &EmailDelivery{} },
	NameEmailBounced:              func() DomainEvent { return &EmailDelivery{} },
	NameEmailComplained:           func() DomainEvent { return &EmailDelivery{} },
	NameMarketingConfirmed:        func() DomainEvent { return &MarketingConfirmed{} },
	NameSmsConfirmed:              func() DomainEvent { return &SmsConfirmed{} },
	NameCollected:                 func() DomainEvent { return &CollectedEvent{} },
	NameContactTagged:             func() DomainEvent { return &ContactTagChange{} },
	NameContactUntagged:           func() DomainEvent { return &ContactTagChange{} },
	NameContactFieldChanged:       func() DomainEvent { return &ContactFieldChanged{} },
	NameAutomationEnrolled:        func() DomainEvent { return &EnrollmentChange{} },
	NameAutomationCompleted:       func() DomainEvent { return &EnrollmentChange{} },
	NameAutomationGoalReached:     func() DomainEvent { return &EnrollmentChange{} },
	NameAutomationExited:          func() DomainEvent { return &EnrollmentChange{} },
	NameTelegramBound:             func() DomainEvent { return &BindingChange{} },
	NameTelegramUnbound:           func() DomainEvent { return &BindingChange{} },
	NameTelegramReceived:          func() DomainEvent { return &TelegramReceived{} },
	NameTelegramSent:              func() DomainEvent { return &TelegramSent{} },
	NameTelegramClicked:           func() DomainEvent { return &TelegramClicked{} },
	NameTelegramUnsubscribed:      func() DomainEvent { return &TelegramUnsubscribed{} },
	NameSmsReceived:               func() DomainEvent { return &SmsReceived{} },
	NameSmsSent:                   func() DomainEvent { return &SmsSent{} },
	NameSmsDelivered:              func() DomainEvent { return &SmsDelivery{} },
	NameSmsUndelivered:            func() DomainEvent { return &SmsDelivery{} },
	NameSmsClicked:                func() DomainEvent { return &SmsClicked{} },
	NameSmsUnsubscribed:           func() DomainEvent { return &SmsUnsubscribed{} },
	NameAuditEntry:                func() DomainEvent { return &AuditEntry{} },
	NameTrackingDomainChanged:     func() DomainEvent { return &TrackingDomainChanged{} },

	NameUserVerificationRequested:  func() DomainEvent { return &LinkRequested{} },
	NameUserPasswordResetRequested: func() DomainEvent { return &LinkRequested{} },
	NameUserEmailChangeRequested:   func() DomainEvent { return &LinkRequested{} },
	NameUserDeletionRequested:      func() DomainEvent { return &LinkRequested{} },
	NameUserAddressClaimed:         func() DomainEvent { return &AddressClaimed{} },
	NameUserSignedUp:               func() DomainEvent { return &UserSignedUp{} },
	NameUserIdentityLinked:         func() DomainEvent { return &IdentityLinked{} },
	NameUserDeleted:                func() DomainEvent { return &UserDeleted{} },
	NameUserPasskeyAdded:           func() DomainEvent { return &PasskeyChange{} },
	NameUserPasskeyRemoved:         func() DomainEvent { return &PasskeyChange{} },

	NameInvitationCreated:                 func() DomainEvent { return &InvitationCreated{} },
	NameWorkspaceSecondFactorRequired:     func() DomainEvent { return &SecondFactorRequired{} },
	NameMembershipSecondFactorReminderDue: func() DomainEvent { return &SecondFactorReminderDue{} },
	NameWorkspaceSuspended:                func() DomainEvent { return &WorkspaceSuspended{} },
	NameWorkspaceAbuseWarned:              func() DomainEvent { return &AbuseWarned{} },
	NameJoinDomainLost:                    func() DomainEvent { return &JoinDomainLost{} },
	NameSendingDomainVerificationLost:     func() DomainEvent { return &SendingDomainVerificationLost{} },
	NameTrackingDomainReplaced:            func() DomainEvent { return &TrackingDomainReplaced{} },
	NameTrackingDomainRemoved:             func() DomainEvent { return &TrackingDomainRemoved{} },
	NameWebhookEndpointDisabled:           func() DomainEvent { return &WebhookEndpointDisabled{} },
}

// Names are the bus types of every event Decode knows, sorted.
func Names() []string {
	return slices.Sorted(maps.Keys(registry))
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
	// before publishing so no address or phone reaches an event job. Empty when there was none.
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
// makes a second publish of the same send a repeat that is dropped — the
// send path sets it (a deterministic per-recipient/per-step key) so a job retry
// can't write a second email.sent row; open/click/unsubscribe leave it empty and
// fall back to the envelope ULID.
type EmailEngagement struct {
	ObservedAt  time.Time `json:"observedAt,omitzero"`
	Action      string    `json:"action"` // email.sent|email.opened|email.clicked|email.unsubscribed
	WorkspaceID int64     `json:"workspaceId"`
	ContactID   int64     `json:"contactId"`
	Email       string    `json:"email"`
	BroadcastID int64     `json:"broadcastId"`
	URL         string    `json:"url,omitempty"`
	DedupID     string    `json:"dedupId,omitempty"`
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
		props[outboundMessageIDProperty] = e.OutboundMessageID
	}
	if e.Captured {
		props[CapturedProperty] = true
	}
	if e.Machine {
		props[MachineProperty] = true
	}
	return Projection{
		OccurredAt: e.ObservedAt,
		Subject:    e.Email, Action: e.Action, Email: e.Email, Properties: props, ContactID: e.ContactID,
		AutomationID: e.AutomationID, BroadcastID: e.BroadcastID, URL: e.URL, Machine: e.Machine, Captured: e.Captured,
	}
}

// EmailDelivery is emitted when a provider reports what became of one of our sends: a
// Delivery notice (ADR 0053) ingested from a provider callback, e.g. SES via SNS. Action is
// the specific name. BounceKind distinguishes a hard bounce from a soft one and is empty for
// the other kinds; only hard bounces and complaints suppress the address. ContactID is 0 when
// the address has no contact in the workspace; OutboundMessageID is 0 when the notice named no
// message or none matched, and the Event then stays at the level of the address.
type EmailDelivery struct {
	ObservedAt  time.Time `json:"observedAt,omitzero"`
	Action      string    `json:"action"` // email.delivered|email.delayed|email.dropped|email.bounced|email.complained
	WorkspaceID int64     `json:"workspaceId"`
	ContactID   int64     `json:"contactId,omitempty"`
	Email       string    `json:"email"`
	BounceKind  string    `json:"bounceKind,omitempty"` // hard|soft (bounces only)
	Provider    string    `json:"provider,omitempty"`   // ses, ...
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
	// part of the Event's namespaced source identity, so a provider redelivery is a repeat.
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
		outboundMessageIDProperty: e.OutboundMessageID,
		"broadcastId":             e.BroadcastID,
		"automationId":            e.AutomationID,
	} {
		if v != 0 {
			props[k] = v
		}
	}
	if e.Captured {
		props[CapturedProperty] = true
	}
	// A notice of delivery is never a trigger nor an end of an Automation (ADR 0053), so its
	// AutomationID only marks whose email it was.
	return Projection{
		OccurredAt: e.ObservedAt,
		Subject:    e.Email, Action: e.Action, Email: e.Email, Properties: props, ContactID: e.ContactID,
		AutomationID: e.AutomationID, BroadcastID: e.BroadcastID, Captured: e.Captured,
	}
}

// MarketingConfirmed is emitted when an email address confirms its subscription
// (double opt-in, ADR 0013). It is the immutable source of truth for the
// confirmation — the derived Confirmation row is the read-model — and carries the
// GDPR Art. 7 proof: when, how (provenance), and the confirming IP. Publishing it
// also lets a welcome automation trigger on "marketing.confirmed". It is email's
// alone: an SMS Confirmation publishes SmsConfirmed, since a trigger is an action with
// no property filter and a welcome email's Automation must not enroll a phone's consent.
type MarketingConfirmed struct {
	WorkspaceID int64  `json:"workspaceId"`
	ContactID   int64  `json:"contactId,omitempty"`
	Email       string `json:"email"`
	Provenance  string `json:"provenance"`   // double_opt_in|grandfathered
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

// SmsConfirmed is emitted when a phone number's SMS Confirmation is recorded (ADR 0077): consent
// the Workspace collected outside sphericon, relayed with its evidence (imported), or a START the
// person texted (keyword). As marketing.confirmed is on email, it is the immutable proof the
// Confirmation row is derived from, never deleted by age (ADR 0019): when, how (provenance), and
// for an imported one the wording the person was shown, its source and the brand.
type SmsConfirmed struct {
	WorkspaceID int64 `json:"workspaceId"`
	// ContactID is the Contact whose phone the number is, 0 when no Contact has it.
	ContactID int64 `json:"contactId,omitempty"`
	// Phone is the confirmed number in E.164.
	Phone      string `json:"phone"`
	Provenance string `json:"provenance"` // keyword|imported
	// The evidence of an imported Confirmation; empty on a keyword one.
	Wording string `json:"wording,omitempty"`
	Source  string `json:"source,omitempty"`
	Brand   string `json:"brand,omitempty"`
}

func (*SmsConfirmed) EventName() string  { return NameSmsConfirmed }
func (*SmsConfirmed) EventVersion() int  { return 1 }
func (e *SmsConfirmed) Workspace() int64 { return e.WorkspaceID }
func (e *SmsConfirmed) Project() Projection {
	props := map[string]any{}
	for k, v := range map[string]string{"provenance": e.Provenance, "wording": e.Wording, "source": e.Source, "brand": e.Brand} {
		if v != "" {
			props[k] = v
		}
	}
	return Projection{Action: NameSmsConfirmed, Phone: e.Phone, Properties: props, ContactID: e.ContactID}
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
	// DedupID is the upstream identity when the client gave it a clientEventId
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
