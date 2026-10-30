package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserTombstone is what the service feed of Users (ADR 0062) keeps of a deleted User: the id
// it had and the feed position its deletion took, so the deletion is an entry of the feed and
// not silence. Nothing else of the User is kept (ADR 0050). created_at is when the User was
// deleted. An ent hook on User (internal/userfeed) writes it in the transaction of the
// deletion; only that hook writes the table, and ee/staff reads it.
type UserTombstone struct {
	ent.Schema
}

func (UserTombstone) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (UserTombstone) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The deleted User's id.
		field.Int64("user_id").
			Unique().
			Immutable().
			Annotations(ScopedRef{
				Unchecked: true,
				Reason:    "tombstone of a deleted User: the entry outlives the User row, because the deletion is what the operator's control plane reads the feed for",
			}),
		// The feed position the deletion took, in the same order as users.feed_position.
		field.Int64("feed_position").
			Immutable(),
	}
}

func (UserTombstone) Indexes() []ent.Index {
	return []ent.Index{
		// The feed pages by (feed_position, user_id).
		index.Fields("feed_position", "user_id"),
	}
}
