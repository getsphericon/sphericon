package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Rule is a stateless reaction to an Event (ADR 0054): when any of its Trigger events is
// recorded for a Contact and its Condition holds, its Actions run at once. The Trigger
// events live in rule_triggers (a Rule may have several, and the schema is plain tables);
// the Condition is the shared tree (internal/condition) stored as JSON; the Actions are
// an ordered JSON array. A Rule has Firings, which are history: it is discarded, and only a
// Rule with none is purged (ADR 0049).
type Rule struct {
	ent.Schema
}

func (Rule) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("rules", Rule{}.Fields()),
		Audited{Action: "rule", NameField: "name"},
	}
}

func (Rule) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "rules"}, DiscardMixin{}}
}

func (Rule) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.String("name").
			NotEmpty(),
		// draft: acts on nothing; active: reacts to Events recorded after activated_at.
		field.Enum("status").
			Values("draft", "active").
			Default("draft"),
		// When the Rule was last activated. A Rule acts only on Events recorded at or after
		// it, so activating never replays history, and deactivating then activating again
		// does not replay what happened in between. Kept when the Rule is deactivated.
		field.Time("activated_at").
			Optional().
			Nillable(),
		// The Condition tree (internal/condition) as JSON; empty means every Contact.
		field.String("condition").
			Optional().
			Nillable(),
		// JSON array of Actions, run in order: [{"type":"apply_tag","tag":"clicked"}].
		// Sensitive: a later Action kind (an HTTP call) holds a URL and a signing secret, which
		// must never reach an Audit diff; the entry records only that the Actions changed.
		field.String("actions").
			Default("[]").
			Annotations(Sensitive{}),
	}
}

func (Rule) Edges() []ent.Edge {
	return []ent.Edge{
		// The Trigger events die with their Rule.
		edge.To("triggers", RuleTrigger.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		// A Firing dies with its Rule's row (the key cascades, for the Workspace's erasure).
		// Rules.Delete purges only a Rule with no Firings, so in use the cascade removes
		// nothing: a Rule with Firings is archived and its Firings stay readable.
		edge.To("firings", RuleFiring.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (Rule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id"),
	}
}

// RuleTrigger is one Trigger event of a Rule: the Event action that makes it fire.
type RuleTrigger struct {
	ent.Schema
}

func (RuleTrigger) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "rule_triggers"},
	}
}

func (RuleTrigger) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "rule_triggers"}}
}

func (RuleTrigger) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("rule_id"),
		// The Event.action that triggers the Rule (e.g. "email.clicked", a custom event).
		field.String("action").
			NotEmpty(),
	}
}

func (RuleTrigger) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("rule", Rule.Type).
			Ref("triggers").
			Field("rule_id").
			Required().
			Unique(),
	}
}

func (RuleTrigger) Indexes() []ent.Index {
	return []ent.Index{
		// The lookup of an Event: the Rules of a Workspace that react to an action.
		index.Fields("workspace_id", "action"),
		// A Rule names an action once; the leading rule_id also serves the foreign key.
		index.Fields("rule_id", "action").
			Unique(),
	}
}

// RuleFiring is one execution of a Rule for one Event (ADR 0054): written when the Rule's
// Condition held, unique per Rule and Event so a redelivered Event does nothing twice. It
// records the result of every Action. It belongs to the Contact and goes with it on Erasure.
type RuleFiring struct {
	ent.Schema
}

func (RuleFiring) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "rule_firings"},
	}
}

func (RuleFiring) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "rule_firings"}}
}

func (RuleFiring) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("rule_id"),
		// The firing dies with its Contact (ON DELETE CASCADE; Erasure deletes it).
		field.Int64("contact_id"),
		// The Event that fired the Rule: its action, and its source id (the DedupKey, else the
		// envelope id: what persist stores as the Event's source_id).
		field.String("action").
			NotEmpty(),
		field.String("source_id").
			NotEmpty(),
		// The Rule's Actions as they were when it fired (same JSON as rules.actions): an edit
		// of the Rule after that does not change what this Firing runs, or what its results
		// index into.
		field.String("actions").
			Default("[]").
			Annotations(Sensitive{}),
		// The Event's properties as JSON, for the Actions to template (an http Action's body sees
		// them as `event.properties`). Personal data: it goes with the Contact on Erasure. Empty
		// when the Event had none or more than events.RulePropertiesBound, which the Rule's Condition
		// then saw as none too.
		field.String("properties").
			Optional().
			Nillable(),
		// JSON array with one result per Action run so far, in the Rule's order:
		// [{"type":"apply_tag","status":"succeeded"}]. The Actions job appends as it goes.
		field.String("results").
			Default("[]"),
		// When the last Action finished. A moment, not a flag: it is set once by the Actions job.
		field.Time("completed_at").
			Optional().
			Nillable(),
	}
}

func (RuleFiring) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("rule", Rule.Type).
			Ref("firings").
			Field("rule_id").
			Required().
			Unique(),
		edge.To("contact", Contact.Type).Field("contact_id").Required().Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (RuleFiring) Indexes() []ent.Index {
	return []ent.Index{
		// At most one Firing per Rule and Event; the leading rule_id also serves the foreign key.
		index.Fields("rule_id", "source_id").
			Unique(),
		// The contact foreign key's index: deleting a Contact finds its Firings by it.
		index.Fields("contact_id").
			StorageKey("rule_firings_contact_id"),
		// The retention prune (ADR 0019) selects the Firings older than the window.
		index.Fields("created_at"),
	}
}
