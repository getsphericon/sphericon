// Package uniquenotpartial is a schema internal/entgen's tests load: unique indexes and fields of a discardable entity, partial, marked or neither.
package uniquenotpartial

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/ent/schema"
)

// Workspace is the tenant root the WorkspaceMixin's edge names.
type Workspace struct{ ent.Schema }

func (Workspace) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("things", Thing.Type),
	}
}

type Thing struct{ ent.Schema }

func (Thing) Mixin() []ent.Mixin {
	return []ent.Mixin{schema.TimeMixin{}, schema.WorkspaceMixin{Ref: "things"}, schema.DiscardMixin{}}
}

func (Thing) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").Unique(),
		field.String("key").Unique().Annotations(schema.UniqueOverAll{Reason: "a key is never given again"}),
		field.String("token").Unique().Annotations(schema.UniqueOverAll{}),
		field.String("name"),
		field.String("slug"),
		field.String("host"),
	}
}

func (Thing) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "name").Unique().StorageKey("things_name"),
		index.Fields("workspace_id", "slug").Unique().StorageKey("things_slug").
			Annotations(entsql.IndexWhere("discarded_at IS NULL")),
		index.Fields("host").Unique().StorageKey("things_host").
			Annotations(schema.UniqueOverAll{Reason: "a host is served by one row, kept or not"}),
		index.Fields("workspace_id", "code"),
	}
}
