package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ConsentRecord is one decision of a User on one published version of a Sign-up item's Legal
// document (ADR 0062): accepted, declined or withdrawn, or shown for a notice, with where it was
// made and the IP address and user agent it came from. Records are only ever added: every field is
// immutable, and the latest record of an item is the User's current answer. It belongs to the
// User, not a Workspace, so it has no scoped wrapper; only internal/consentrecords writes and
// reads it. It is deleted with the User (the foreign key cascades, ADR 0050). created_at is when
// the decision was made. The item is the version's: it is not copied onto the record.
type ConsentRecord struct {
	ent.Schema
}

func (ConsentRecord) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("consent_records", ConsentRecord{}.Fields()),
	}
}

func (ConsentRecord) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (ConsentRecord) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("user_id").
			Immutable(),
		field.Int64("version_id").
			Immutable(),
		field.Enum("decision").
			Values("accepted", "declined", "withdrawn", "shown").
			Immutable(),
		// Where the decision was made.
		field.Enum("source").
			Values("sign_up_form", "provider_screen", "invitation", "re_acceptance", "account_settings").
			Immutable(),
		// The client's address, resolved as the rate limiter resolves it (internal/clientip).
		field.String("ip").
			Optional().
			Nillable().
			Immutable(),
		field.String("user_agent").
			Optional().
			Nillable().
			Immutable(),
	}
}

func (ConsentRecord) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("consent_records").
			Field("user_id").
			Unique().
			Required().
			Immutable(),
		// A published version is never deleted, so a record always shows what was decided on.
		edge.From("version", LegalDocumentVersion.Type).
			Ref("consent_records").
			Field("version_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (ConsentRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "id"),
		index.Fields("version_id"),
	}
}
