package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Usage is one materialized figure of a Workspace: the value of one metric over one
// Usage period (GLOSSARY, ADR 0009). While the period is open the row is its Live
// usage (finalized_at unset, refreshed by the usage worker); at period close the same row
// becomes the Usage snapshot (finalized_at set) and never changes again. Only ee/usage
// writes and reads it. A new metric needs a Metric constant in ee/usage and no schema
// change: the metric is a plain string, one row per (Workspace, period, metric).
type Usage struct {
	ent.Schema
}

func (Usage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "usages"},
	}
}

func (Usage) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "usages"}}
}

func (Usage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The first instant of the Usage period (the calendar month, UTC).
		field.Time("period_start").
			Immutable(),
		// The metric name, e.g. emails_sent.
		field.String("metric").
			NotEmpty().
			Immutable(),
		field.Int64("value").
			NonNegative().
			Default(0),
		// The moment the period closed and the row became a Usage snapshot, which is
		// immutable; nil while the row is Live usage.
		field.Time("finalized_at").
			Optional().
			Nillable(),
		// When the value was last computed.
		field.Time("computed_at"),
	}
}

func (Usage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "period_start", "metric").
			Unique(),
		// Paging every Workspace's figures of one period.
		index.Fields("period_start", "workspace_id"),
	}
}
