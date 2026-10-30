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
// recorded for a Contact and its Condition holds, its Actions run at once. The definition
// (Trigger events, Condition, Actions) lives in its Rule versions (ADR 0065): the Rule points
// at its live version, what it does, and at its draft, the unpublished one above live. The
// name, the status and archiving stay on the Rule. A Rule has Firings, which are history: it
// is discarded, and only a Rule with none is purged (ADR 0049).
type Rule struct {
	ent.Schema
}

func (Rule) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("rules", Rule{}.Fields()),
		Audited{Action: "rule", NameField: nameField},
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
		field.String(nameField).
			NotEmpty(),
		// inactive: acts on nothing; active: reacts to Events recorded after activated_at. Publish
		// makes a Rule active, Deactivate inactive (ADR 0065).
		field.Enum("status").
			Values("active", "inactive").
			Default("inactive"),
		// When the Rule last became active by Publish. A Rule acts only on Events recorded at or
		// after it, so publishing never replays history, and deactivating then publishing again
		// does not replay what happened in between. Kept when the Rule is deactivated.
		field.Time("activated_at").
			Optional().
			Nillable(),
		// The version the Rule runs; nil until it is first published.
		field.Int64("live_version_id").
			Optional().
			Nillable(),
		// The unpublished version, the one above live; nil when there are no unpublished changes.
		field.Int64("draft_version_id").
			Optional().
			Nillable(),
	}
}

func (Rule) Edges() []ent.Edge {
	return []ent.Edge{
		// Purging a Rule (only one with no Firings) purges its versions with it.
		edge.To("versions", RuleVersion.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		// The two pointers into the versions. SET NULL breaks the circle of keys between a Rule and
		// its versions: a purge of the Rule cascades to the versions it points at.
		edge.To("live_version", RuleVersion.Type).
			Field("live_version_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.To("draft_version", RuleVersion.Type).
			Field("draft_version_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
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
		index.Fields("live_version_id"),
		index.Fields("draft_version_id"),
	}
}

// RuleVersion is one copy of a Rule's definition (ADR 0065, GLOSSARY: Rule version): its Trigger
// events (rule_version_triggers), its Condition and its Actions. The live version is never
// rewritten; the draft is rewritten in place until it is published. A Firing references the
// version that fired. Not audited itself: the Rule's own entries say a draft was made, published
// or reverted (its pointers move).
type RuleVersion struct {
	ent.Schema
}

func (RuleVersion) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "rule_versions"},
	}
}

func (RuleVersion) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "rule_versions"}}
}

func (RuleVersion) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("rule_id"),
		// 1 for the first, plus one per version after it, within one Rule.
		field.Int("version").
			Positive().
			Immutable(),
		// The Condition tree (internal/condition) as JSON; empty means every Contact.
		field.String("condition").
			Optional().
			Nillable(),
		// JSON array of Actions, run in order: [{"type":"apply_tag","tag":"clicked"}].
		// Sensitive: an http Action holds a sealed URL and signing secret.
		field.String("actions").
			Default("[]").
			Annotations(Sensitive{}),
	}
}

func (RuleVersion) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("rule", Rule.Type).
			Ref("versions").
			Field("rule_id").
			Required().
			Unique(),
		// The Trigger events die with their version.
		edge.To("triggers", RuleVersionTrigger.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		// The Firings of the version. Only a draft is ever deleted alone (Revert), and no Firing
		// stands on a draft, so the key never has to act.
		edge.To("firings", RuleFiring.Type).
			Annotations(entsql.OnDelete(entsql.NoAction)),
	}
}

func (RuleVersion) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "rule_id", "version").
			Unique().
			StorageKey("rule_versions_workspace_id_rule_id_version"),
		index.Fields("rule_id"),
	}
}

// RuleVersionTrigger is one Trigger event of a Rule version: the Event action that makes it fire.
type RuleVersionTrigger struct {
	ent.Schema
}

func (RuleVersionTrigger) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "rule_version_triggers"},
	}
}

func (RuleVersionTrigger) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "rule_version_triggers"}}
}

func (RuleVersionTrigger) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("version_id"),
		// The Event.action that triggers the version (e.g. "email.clicked", a custom event).
		field.String("action").
			NotEmpty(),
	}
}

func (RuleVersionTrigger) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("version", RuleVersion.Type).
			Ref("triggers").
			Field("version_id").
			Required().
			Unique(),
	}
}

func (RuleVersionTrigger) Indexes() []ent.Index {
	return []ent.Index{
		// The lookup of an Event: the versions of a Workspace that react to an action.
		index.Fields("workspace_id", "action"),
		// A version names an action once; the leading version_id also serves the foreign key.
		index.Fields("version_id", "action").
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
		// The Rule version that fired: its Actions are what this Firing runs and what its results
		// index into. A version that has fired is never rewritten (ADR 0065). Every Firing has
		// one; the column stays nullable because a NOT NULL would lock the live table (ADR 0033).
		field.Int64("version_id").
			Optional(),
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
		edge.From("version", RuleVersion.Type).
			Ref("firings").
			Field("version_id").
			Unique(),
	}
}

func (RuleFiring) Indexes() []ent.Index {
	return []ent.Index{
		// At most one Firing per Rule and Event; the leading rule_id also serves the foreign key.
		index.Fields("rule_id", "source_id").
			Unique(),
		// The Workspace foreign key's index: erasing a Workspace finds its Firings by it.
		index.Fields("workspace_id"),
		// The contact foreign key's index: deleting a Contact finds its Firings by it.
		index.Fields("contact_id").
			StorageKey("rule_firings_contact_id"),
		// The retention prune (ADR 0019) selects the Firings older than the window.
		index.Fields("created_at"),
		// The version foreign key's index.
		index.Fields("version_id"),
	}
}
