package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	watermillsql "github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/suppression"
)

// Enroller enrolls a contact into automations matching an event action. The
// automations subscriber delegates to it, keeping durable execution in river
// (the implementation enqueues an EvaluateTrigger job).
type Enroller interface {
	OnEvent(ctx context.Context, workspaceID, contactID int64, action string, sourceAutomationID int64) error
}

// RuleEvent is an Event a Rule may react to, as the Rules consumer hands it to the queue: the
// Contact it is for, its action, the id a Firing is unique on (the DedupKey, else the envelope
// id: what persist stores as source_id), when it was recorded and the properties it carries.
type RuleEvent struct {
	WorkspaceID int64
	ContactID   int64
	Action      string
	SourceID    string
	RecordedAt  time.Time
	Properties  map[string]any
}

// RuleEvaluator queues the evaluation of an Event against the Workspace's Rules. The Rules
// subscriber delegates to it, keeping durable execution in river (the implementation enqueues
// an evaluation job), like Enroller does for Automations.
type RuleEvaluator interface {
	OnRuleEvent(ctx context.Context, ev RuleEvent) error
}

// WebhookDispatcher fans an event out to the workspace's matching webhook
// endpoints. The webhooks subscriber delegates to it, keeping durable delivery
// (per-endpoint jobs, retries) in river.
type WebhookDispatcher interface {
	Dispatch(ctx context.Context, s *ent.Scoped, eventName, deliveryID string, body []byte) error
}

// NewRouter builds the watermill router that hosts the domain-event subscribers.
// The events package owns its full consume-side runtime (router + per-group
// subscribers); the producer side is Bus. Run the returned router in a goroutine
// and Close it on shutdown.
func NewRouter() (*message.Router, error) {
	router, err := message.NewRouter(message.RouterConfig{}, watermill.NewSlogLogger(slog.Default()))
	if err != nil {
		return nil, err
	}
	// Per-handler OTel spans + metrics (global providers; no-op when disabled).
	router.AddMiddleware(otelMiddleware())
	return router, nil
}

// Consumer groups of the domain-events outbox. Each registered subscriber owns
// one group (its own offset cursor), and the outbox prune reads this registry to
// know whose cursors bound what it may delete, so a new subscriber is added here
// and nowhere else.
const (
	GroupPersist     = "persist"
	GroupAutomations = "automations"
	GroupRules       = "rules"
	GroupWebhooks    = "webhooks"
	GroupSuppression = "suppression"
	// GroupAudit is the Enterprise audit subscriber's group (ee/audit). Core lists it
	// so the outbox prune never deletes an Audit entry the subscriber has not consumed
	// (ADR 0022); the subscriber is always registered, licensed or not.
	GroupAudit = "audit"
	// GroupExport is the Enterprise Export streams' subscriber group (ee/export), listed
	// for the same reason as GroupAudit (ADR 0045).
	GroupExport = "export"
)

// coreGroups are the groups core wires itself, in registration order.
func coreGroups() []string {
	return []string{GroupPersist, GroupAutomations, GroupRules, GroupWebhooks, GroupSuppression}
}

// ConsumerGroups lists every consumer group registered in code, in registration
// order: the core groups, then the extra consumers' (RegisterSubscribers).
func ConsumerGroups() []string {
	return append(coreGroups(), GroupAudit, GroupExport)
}

// RegisterSubscribers wires the domain-event consumers onto the shared watermill
// router. Each consumer gets its OWN subscriber with a distinct consumer group
// (see ConsumerGroups), so every subscriber receives every event (fan-out) rather
// than competing for messages. Add new subscribers to the table below and to
// ConsumerGroups without touching producers.
//
// extra are consumers owned outside core (the Enterprise audit subscriber, ee/): each
// gets its own consumer group like the built-in ones, and its group is listed in
// ConsumerGroups so the outbox prune waits for it.
//
// opts tune every subscriber it builds (see SubscribeOption).
func RegisterSubscribers(router *message.Router, db *sql.DB, client *ent.Client, enroller Enroller, rules RuleEvaluator, dispatcher WebhookDispatcher, extra []Consumer, opts ...SubscribeOption) error {
	handlers := map[string]struct {
		name    string
		handler message.NoPublishHandlerFunc
	}{
		GroupPersist:     {"persist_event", persistConsumer(client)},
		GroupAutomations: {"enroll_automations", AutomationsConsumer(enroller)},
		GroupRules:       {"evaluate_rules", RulesConsumer(rules)},
		GroupWebhooks:    {"dispatch_webhooks", WebhooksConsumer(client, dispatcher)},
		GroupSuppression: {"update_suppression", suppressionConsumer(client)},
	}
	for _, group := range coreGroups() {
		h := handlers[group]
		sub, err := NewSubscriber(db, group, opts...)
		if err != nil {
			return fmt.Errorf("%s subscriber: %w", group, err)
		}
		router.AddConsumerHandler(h.name, TopicDomainEvents, sub, h.handler)
	}
	for _, c := range extra {
		sub, err := NewSubscriber(db, c.Name, opts...)
		if err != nil {
			return fmt.Errorf("%s subscriber: %w", c.Name, err)
		}
		router.AddConsumerHandler(c.Name, TopicDomainEvents, sub, c.Handler())
	}
	return nil
}

// Consumer is a bus subscriber registered from outside the events package. Name is
// both its router handler name and its consumer group.
type Consumer struct {
	Name   string
	Handle func(ctx context.Context, env Envelope) error
}

// Handler adapts the consumer to a watermill handler that decodes the envelope.
func (c Consumer) Handler() message.NoPublishHandlerFunc {
	return func(msg *message.Message) error {
		var env Envelope
		if err := json.Unmarshal(msg.Payload, &env); err != nil {
			return fmt.Errorf("unmarshal envelope: %w", err)
		}
		return c.Handle(msg.Context(), env)
	}
}

// suppressionConsumer adds the event's address to the workspace suppression list
// when the action implies it (hard bounce, complaint).
func suppressionConsumer(client *ent.Client) message.NoPublishHandlerFunc {
	return func(msg *message.Message) error {
		var env Envelope
		if err := json.Unmarshal(msg.Payload, &env); err != nil {
			return fmt.Errorf("unmarshal envelope: %w", err)
		}
		return Suppress(msg.Context(), client, env)
	}
}

// Suppress adds the event's destination to the workspace suppression list when
// the action implies a global do-not-send (hard bounce, complaint). No-op for
// other actions or a missing email. Idempotent — a redelivered or repeated event
// keeps the original entry rather than erroring or overwriting its reason.
// Exported so the logic can be tested without running the router.
func Suppress(ctx context.Context, client *ent.Client, env Envelope) error {
	ev, err := Decode(env)
	if err != nil {
		return err
	}
	p := ev.Project()
	kind, _ := p.Properties["bounceKind"].(string)
	reason, ok := suppression.ForEvent(p.Action, kind)
	if !ok {
		return nil
	}
	// The Workspace comes from the event envelope: a consumer has no membership or
	// api token, so this is a scope source outside the site/external/job list.
	s := client.Scoped(env.WorkspaceID)
	err = suppression.Add(ctx, s, suppression.Entry{Destination: p.Email, Reason: reason, ContactID: p.ContactID})
	if errors.Is(err, suppression.ErrDestinationEmpty) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("suppress %q: %w", p.Email, err)
	}
	return nil
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

// WebhooksConsumer serializes the event into the public webhook body and hands it to the
// dispatcher. The bus subscriber is a raw-client allowlist entry (ADR 0017): an event envelope
// carries only a Workspace id, so the scope handed to the dispatcher is built here.
func WebhooksConsumer(client *ent.Client, dispatcher WebhookDispatcher) message.NoPublishHandlerFunc {
	return func(msg *message.Message) error {
		var env Envelope
		if err := json.Unmarshal(msg.Payload, &env); err != nil {
			return fmt.Errorf("unmarshal envelope: %w", err)
		}
		ev, err := Decode(env)
		if err != nil {
			return err
		}
		if IsBackfill(ev) {
			return nil // loaded history: recorded, never delivered
		}
		p := ev.Project()
		// Filter/route on the semantic action (e.g. "page_view"), not the bus type. An
		// unprojected event (an Audit entry) has no Event action: it is forwarded under
		// its bus type, only to endpoints that select it, and the edition's dispatcher
		// gates it by license (ADR 0022).
		name := p.Action
		if _, unprojected := ev.(Unprojected); unprojected {
			name = env.Name
		}
		data := env.Data
		if entry, ok := ev.(*AuditEntry); ok {
			// The staff identity never reaches a customer's endpoint (ADR 0022).
			entry.MaskStaff()
			if data, err = json.Marshal(entry); err != nil {
				return fmt.Errorf("marshal audit entry: %w", err)
			}
		}
		// A retried event whose repeat slipped past the collect-side check carries the same
		// DedupKey, so the receiver sees one id for it and can drop the second delivery.
		id := env.ID
		if env.DedupKey != "" {
			id = env.DedupKey
		}
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
			return fmt.Errorf("marshal webhook payload: %w", err)
		}
		return dispatcher.Dispatch(msg.Context(), client.Scoped(env.WorkspaceID), name, id, body)
	}
}

// AutomationsConsumer enrolls the event's contact into matching automations. It
// skips events with no contact (ContactID == 0, e.g. anonymous collect events).
func AutomationsConsumer(enroller Enroller) message.NoPublishHandlerFunc {
	return func(msg *message.Message) error {
		var env Envelope
		if err := json.Unmarshal(msg.Payload, &env); err != nil {
			return fmt.Errorf("unmarshal envelope: %w", err)
		}
		ev, err := Decode(env)
		if err != nil {
			return err
		}
		if _, skip := ev.(Unprojected); skip {
			return nil // an administrative action is never an automation trigger
		}
		if IsBackfill(ev) {
			return nil // loaded history never enrolls a contact
		}
		p := ev.Project()
		if p.ContactID == 0 {
			return nil // no contact to enroll (e.g. an anonymous collected event)
		}
		if p.Machine {
			return nil // software opened or clicked: nobody is enrolled by a scanner (ADR 0053)
		}
		// Enroll on the semantic action so a trigger can be "contact.created" or a
		// customer action like "page_view".
		return enroller.OnEvent(msg.Context(), env.WorkspaceID, p.ContactID, p.Action, p.AutomationID)
	}
}

// RulePropertiesBound is the most JSON an Event's properties may weigh to be shown to a Rule.
// Ingest already refuses larger customer events; a larger one reaching a Rule anyway has no
// properties, for the Condition and for the Firing's Actions alike, so a Rule never decides on a
// value its Actions could not template and a job row does not grow without limit.
const RulePropertiesBound = 32 << 10

// RuleProperties is the view of an Event's properties that a Rule evaluates and a Firing keeps:
// the properties themselves, or nil when they weigh more than RulePropertiesBound. Every path
// that shows properties to a Rule (the consumer, Evaluate, a dry run) goes through it.
func RuleProperties(props map[string]any) map[string]any {
	if len(props) == 0 {
		return props
	}
	if encoded, err := json.Marshal(props); err != nil || len(encoded) > RulePropertiesBound {
		return nil
	}
	return props
}

// machineEvent reports whether ev was caused by a machine (a mail scanner's open or click, ADR
// 0053), which never triggers a Rule.
func machineEvent(ev DomainEvent) bool { return ev.Project().Machine }

// RulesConsumer queues the evaluation of the event against the Workspace's Rules (ADR 0054).
// Like the Automations consumer it skips the events that are never a trigger, and it also skips
// an event with no Contact, a machine event and an event a Rule's Action published (its
// envelope carries CausedBy).
func RulesConsumer(rules RuleEvaluator) message.NoPublishHandlerFunc {
	return func(msg *message.Message) error {
		var env Envelope
		if err := json.Unmarshal(msg.Payload, &env); err != nil {
			return fmt.Errorf("unmarshal envelope: %w", err)
		}
		if env.CausedBy != "" {
			return nil // an Action's own event: Rules never chain
		}
		ev, err := Decode(env)
		if err != nil {
			return err
		}
		if _, skip := ev.(Unprojected); skip {
			return nil
		}
		if IsBackfill(ev) {
			return nil // loaded history never fires a Rule
		}
		if machineEvent(ev) {
			return nil
		}
		p := ev.Project()
		if p.ContactID == 0 {
			return nil // no Contact to act on
		}
		sourceID := env.DedupKey
		if sourceID == "" {
			sourceID = env.ID
		}
		recorded := env.RecordedAt
		if recorded.IsZero() {
			recorded = env.OccurredAt
		}
		return rules.OnRuleEvent(msg.Context(), RuleEvent{
			WorkspaceID: env.WorkspaceID,
			ContactID:   p.ContactID,
			Action:      p.Action,
			SourceID:    sourceID,
			RecordedAt:  recorded,
			Properties:  RuleProperties(p.Properties),
		})
	}
}

// NewSubscriber builds a watermill-sql subscriber for the outbox topic under a
// dedicated consumer group (its own offset cursor). InitializeSchema is false: the
// migrations create the message + offsets tables.
func NewSubscriber(db *sql.DB, consumerGroup string, opts ...SubscribeOption) (message.Subscriber, error) {
	cfg := watermillsql.SubscriberConfig{
		SchemaAdapter:    outboxSchema,
		OffsetsAdapter:   outboxOffsets,
		InitializeSchema: false,
		ConsumerGroup:    consumerGroup,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return watermillsql.NewSubscriber(watermillsql.BeginnerFromStdSQL(db), cfg, watermill.NewSlogLogger(slog.Default()))
}

// SubscribeOption adjusts a subscriber's watermill configuration. Production uses none:
// the default schema adapter only delivers a row once every transaction that began before
// it has finished (see testhelper.PrivateOutbox for the one reason to change that).
type SubscribeOption func(*watermillsql.SubscriberConfig)

// WithSchemaAdapter replaces the adapter that builds the subscriber's select query. The
// adapter must keep the outbox table layout of the producer.
func WithSchemaAdapter(a watermillsql.SchemaAdapter) SubscribeOption {
	return func(cfg *watermillsql.SubscriberConfig) { cfg.SchemaAdapter = a }
}

func persistConsumer(client *ent.Client) message.NoPublishHandlerFunc {
	return func(msg *message.Message) error {
		var env Envelope
		if err := json.Unmarshal(msg.Payload, &env); err != nil {
			return fmt.Errorf("unmarshal envelope: %w", err)
		}
		return Persist(msg.Context(), client, env)
	}
}

// Persist writes the durable Event-projection row for a domain event. It is the
// one place the engagement log is written, replacing scattered Event.Create
// calls. Exported so the projection logic can be unit-tested directly without
// running the router.
//
// It decodes the typed event and writes the row from its Project(), so any event
// type projects faithfully without persist knowing the concrete fields.
func Persist(ctx context.Context, client *ent.Client, env Envelope) error {
	ev, err := Decode(env)
	if err != nil {
		return err
	}
	if _, skip := ev.(Unprojected); skip {
		return nil // not an Event: no row, so no segment or trigger can see it
	}
	p := ev.Project()

	create := client.Event.Create().
		SetWorkspaceID(env.WorkspaceID).
		SetSubjectID(p.Subject).
		SetAction(p.Action).
		SetProperties(p.Properties).
		SetOccurredAt(env.OccurredAt).
		SetBackfill(IsBackfill(ev))
	// Dedup on the DedupKey when the event carries a natural upstream id, else the
	// envelope ULID. Empty ⇒ leave source_id NULL (distinct under the unique index)
	// rather than "", so a missing id never collides and false-dedupes a distinct event.
	sourceID := env.ID
	if env.DedupKey != "" {
		sourceID = env.DedupKey
	}
	if sourceID != "" {
		create.SetSourceID(sourceID)
	}
	// The authoritative Contact link, resolved at ingest. 0 ⇒ anonymous (leave null).
	// Delivery is asynchronous, so the Contact may be gone by now (erased, deleted): the
	// Event outlives it unlinked rather than violating the contact_id FK.
	if p.ContactID != 0 {
		linked, err := client.Contact.Query().
			Where(contact.ID(p.ContactID), contact.WorkspaceID(env.WorkspaceID)).Exist(ctx)
		if err != nil {
			return fmt.Errorf("resolve contact of event %q: %w", env.Name, err)
		}
		if linked {
			create.SetContactID(p.ContactID)
		}
	}
	if p.VisitorID != "" {
		create.SetVisitorID(p.VisitorID)
	}
	if p.Email != "" {
		create.SetEmail(p.Email)
	}
	if p.Phone != "" {
		create.SetPhone(p.Phone)
	}
	// Idempotent: at-least-once delivery can redeliver an envelope after a crash,
	// and SNS redelivers notifications with the same DedupKey; dedupe on the
	// source_id so the projection row is written at most once. ent reports a
	// conflict skipped by DoNothing as sql.ErrNoRows, which is how a first write
	// is told from a duplicate: the bounce/complaint outcome counter ticks only on
	// the first.
	err = create.OnConflictColumns(event.FieldSourceID).DoNothing().Exec(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("persist event %q: %w", env.Name, err)
	}
	recordProviderOutcome(ctx, ev)
	return nil
}

// recordProviderOutcome counts a newly persisted provider bounce or complaint on
// the send-outcome counter. Counting here, at the one place a duplicate is
// detected, keeps SNS redeliveries from counting twice.
func recordProviderOutcome(ctx context.Context, ev DomainEvent) {
	f, ok := ev.(*EmailDelivery)
	if !ok || f.Provider == "" {
		return
	}
	var status messaging.SendStatus
	switch f.Action {
	case NameEmailBounced:
		status = messaging.SendBounce
	case NameEmailComplained:
		status = messaging.SendComplaint
	default:
		return
	}
	// Provider is the Preset's name (ADR 0037), or "ses" for the SES adapter.
	messaging.RecordSendOutcome(ctx, messaging.Provider(f.Provider), status)
}
