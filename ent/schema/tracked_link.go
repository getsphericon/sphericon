package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TrackedLink is the short link `/c/<token>` that stands for one URL of one Outbound message
// (ADR 0083): the Telegram renderer rewrites every link and button URL it emits into one, the SMS
// renderer every link of the text. The token is a short random value and the capability itself, so
// it is stored in the clear; the URL is what a click redirects to. A message has one row per
// distinct URL, which a retry of the same message reuses. It dies with its Outbound message.
//
// A link with no Outbound message is a Test send's: the Test send records no message, yet its text
// shows links in the form that goes out. Following one redirects and records nothing. The Workspace
// has one per distinct URL, which every later Test send of that URL reuses, so they stay as few as
// the URLs tested; they go with the Workspace.
type TrackedLink struct {
	ent.Schema
}

func (TrackedLink) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "tracked_links"}}
}

func (TrackedLink) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "tracked_links"}}
}

func (TrackedLink) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// Nil on a Test send's link.
		field.Int64("outbound_message_id").
			Optional().
			Nillable().
			Immutable(),
		// Unique across Workspaces: a click names the token alone and finds its Workspace from it.
		field.String("token").
			NotEmpty().
			Unique().
			Immutable(),
		// The absolute http(s) URL the link redirects to, as the message carried it.
		field.Text("url").
			NotEmpty().
			Immutable(),
	}
}

func (TrackedLink) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("outbound_message", OutboundMessage.Type).
			Ref("tracked_links").
			Field("outbound_message_id").
			Unique().
			Immutable(),
	}
}

func (TrackedLink) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "outbound_message_id").
			StorageKey("tracked_links_workspace_id_outbound_message_id"),
	}
}
