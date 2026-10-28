package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ExpirableEventPredicate selects the Events that expire by age (ADR 0019): every
// Event except the evidentiary ones (consent proof, complaints, unsubscribes and
// hard bounces). A bounce with no bounceKind counts as soft. It is the
// single source of truth for both the partial index below and the retention
// delete (internal/events.PruneEvents), so the delete can always use the index.
const ExpirableEventPredicate = `action NOT IN ('marketing.confirmed', 'email.complained', 'email.unsubscribed') AND NOT (action = 'email.bounced' AND COALESCE(properties->>'bounceKind', '') = 'hard')`

type Event struct {
	ent.Schema
}

func (Event) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "events"},
	}
}

func (Event) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "events"}}
}

func (Event) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// source_id is the originating domain-event id (ULID) for rows written by
		// the events bus persist subscriber. Unique so at-least-once redelivery
		// dedupes via upsert. Nillable: rows written outside the bus (fixtures)
		// leave it null, and Postgres treats nulls as distinct.
		field.String("source_id").
			Optional().
			Nillable().
			Unique(),
		// The authoritative link to the Contact, resolved by stable identity at ingest
		// (never by email). A nullable FK with ON DELETE SET NULL: Events are immutable
		// and append-only and outlive the Contact they were about. Null for anonymous
		// events before Identify; backfilled (stitched) onto the Contact at Identify.
		field.Int64("contact_id").
			Optional().
			Nillable(),
		// The anonymous device this event came from. Kept so pre-Identify anonymous
		// events can be stitched onto a Contact by visitor_id when Identify arrives.
		field.String("visitor_id").
			Optional().
			Nillable(),
		// Denormalized identity snapshot, for debugging only — the authoritative link
		// is contact_id. May be empty for anonymous events.
		field.String("subject_id").
			Optional(),
		field.String("email").
			Optional().
			Nillable(),
		field.String("phone").
			Optional().
			Nillable(),
		field.String("action").
			NotEmpty(),
		field.JSON("properties", map[string]any{}).
			Optional(),
		field.Time("occurred_at").
			Optional().
			Nillable(),
		// Loaded as history (a Backfill): recorded, but no Automation enrolls, no Webhook
		// receives it and no Rule fires on it.
		field.Bool("backfill").
			Default(false),
	}
}

func (Event) Edges() []ent.Edge {
	return []ent.Edge{
		// The Contact the Event is about; backs the has-event segment conditions
		// (contact.HasEventsWith).
		edge.From("contact", Contact.Type).
			Ref("events").
			Field("contact_id").
			Unique(),
	}
}

func (Event) Indexes() []ent.Index {
	return []ent.Index{
		// Backs event-based segment conditions: the correlated EXISTS joins the
		// Contact to its Events by the stable contact_id (not email) and filters by
		// the event type.
		index.Fields("workspace_id", "contact_id", "action"),
		// Backs stitching anonymous events onto a Contact at Identify time.
		index.Fields("workspace_id", "visitor_id"),
		index.Fields("contact_id"),
		// Backs the Events Export stream (ADR 0045): the keyset page over
		// (updated_at, id) inside one Workspace, so a deep back-fill never scans.
		index.Fields("workspace_id", "updated_at", "id"),
		// Backs the Event retention delete (ADR 0019): created_at is the age
		// column (never null, unlike occurred_at). Partial: evidentiary Events are
		// never deleted by age, so they stay out of the index. The predicate is shared
		// with the retention delete (ExpirableEventPredicate).
		index.Fields("created_at").
			StorageKey("events_created_at_analytical_idx").
			Annotations(entsql.IndexWhere(ExpirableEventPredicate)),
	}
}
