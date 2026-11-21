package schema

import (
	"encoding/json"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// ConditionSpike is a SPIKE (#1674, ADR 0181), never merged: a Condition of one place stored as
// the typed tree's JSON and read back, the stand-in for the stored shape #1676 decides.
type ConditionSpike struct {
	ent.Schema
}

func (ConditionSpike) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "condition_spikes", Checks: map[string]string{
			"condition_spikes_condition_check": conditionCheck("condition"),
		}},
	}
}

func (ConditionSpike) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "condition_spikes"}}
}

func (ConditionSpike) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The place whose tree the condition is: segment or rule.
		field.String("place").
			NotEmpty(),
		// The Condition as the place's typed tree, jsonb.
		field.JSON("condition", json.RawMessage{}),
	}
}
