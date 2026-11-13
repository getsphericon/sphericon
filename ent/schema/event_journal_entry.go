package schema

import (
	"encoding/json"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// EventJournalEntry records an accepted publication independently of River's retention.
// Only explicitly retained, anonymous consumer inputs enter inputs, never the envelope.
// A null Workspace is an Instance fact; the bus owns both scopes (ADR 0116).
type EventJournalEntry struct{ ent.Schema }

func (EventJournalEntry) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "event_journal_entries"}}
}

func (EventJournalEntry) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (EventJournalEntry) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Immutable(),
		field.String("event_id").NotEmpty().Immutable().Unique(),
		field.String("name").NotEmpty().Immutable(),
		field.Int("version").Positive().Immutable(),
		field.Int64(WorkspaceColumn).Optional().Nillable().Immutable().
			Annotations(NotWorkspaceOwned{Reason: "a null Workspace is an Instance fact: the bus owns both scopes (ADR 0116)"}),
		field.Time("occurred_at").Immutable().Annotations(Attribute(reasonHistory)),
		field.Time("recorded_at").Immutable().Annotations(Attribute(reasonHistory)),
		field.JSON("inputs", json.RawMessage{}).Optional().Immutable(),
	}
}

func (EventJournalEntry) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("workspace", Workspace.Type).Ref("event_journal_entries").
			Field("workspace_id").Unique().Immutable().Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (EventJournalEntry) Indexes() []ent.Index {
	return []ent.Index{index.Fields("workspace_id", "recorded_at", "id")}
}
