package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkspaceStanding is what the service keeps about one Workspace for itself (ADR 0168): its place
// in the service feed, when its Setup checklist was found complete and when its owners were told of
// abuse. Jobs and hooks write it, so it lives beside the Workspace and moves no revision of it; one
// row per Workspace, made with it. It is the Workspace's own, as its columns were, not something
// the Workspace owns: the erasure of a Workspace keeps it with the row it erases down to.
type WorkspaceStanding struct {
	ent.Schema
}

func (WorkspaceStanding) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "workspace_standing"},
	}
}

func (WorkspaceStanding) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (WorkspaceStanding) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64(WorkspaceColumn).
			Immutable().
			Annotations(NotWorkspaceOwned{
				Reason: "the standing is the Workspace's own, kept with its row when the erasure removes what it owns",
			}),
		// The Workspace's place in the service feed (ADR 0009): a position that only
		// grows, taken whenever the Workspace is created or an owner is added, removed,
		// transferred or changes email or verification. 0 until the first change. Set
		// only by internal/workspacefeed.
		field.Int64("feed_position").
			Default(0),
		// When the Setup checklist was first found complete. Stamped once, by the read that first
		// finds every step done, and never cleared: after it the overview shows no checklist, and a
		// step that regresses (a domain that loses its record) is an item that needs attention.
		field.Time("setup_completed_at").
			Annotations(Latch()).
			Optional().
			Nillable(),
		// When the owners were last told of the current warning / of an automatic freeze
		// (ADR 0026). The warning marker is cleared once the Workspace leaves the
		// warning tier, which is what makes one email per episode.
		field.Time("abuse_warning_notified_at").
			Annotations(Attribute(reasonNotice)).
			Optional().
			Nillable(),
		field.Time("abuse_freeze_notified_at").
			Annotations(Attribute(reasonNotice)).
			Optional().
			Nillable(),
	}
}

func (WorkspaceStanding) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("workspace", Workspace.Type).
			Ref("standing").
			Field(WorkspaceColumn).
			Unique().
			Required().
			Immutable(),
	}
}

func (WorkspaceStanding) Indexes() []ent.Index {
	return []ent.Index{
		// The feed pages by (feed_position, workspace_id).
		index.Fields("feed_position", "workspace_id"),
	}
}
