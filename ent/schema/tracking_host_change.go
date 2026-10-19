package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TrackingHostChange is one entry of the change log behind the service read of Tracking
// domain hosts (ADR 0038): a host entering dns_verified or serving, or leaving them
// (back to pending, its Sending domain deleted or renamed).
// The id is the position in the log. An ent hook on SendingDomain
// (internal/trackinghostlog) writes it in the transaction of the change, so no write
// path can forget it, and takes an advisory lock held to the end of the transaction so
// positions commit in the order they are taken: a reader that passed a position cannot
// miss an entry committed below it. Only that hook writes the table; ee/staff reads it.
// It has no Workspace edge: an entry must outlive the Workspace row it reports, because
// the removal of a host is what a platform service reads it for.
type TrackingHostChange struct {
	ent.Schema
}

func (TrackingHostChange) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tracking_host_changes"},
	}
}

func (TrackingHostChange) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The Workspace that held the host at this change.
		field.Int64("workspace_id").
			Immutable(),
		// The Sending domain; the host is track.<domain>.
		field.String("domain").
			NotEmpty().
			Immutable(),
		// The state the host entered, or removed when it left dns_verified and serving.
		field.Enum("state").
			Values("dns_verified", "serving", "removed").
			Immutable(),
	}
}

// The creation time is when the change was recorded.
func (TrackingHostChange) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (TrackingHostChange) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("domain"),
	}
}
