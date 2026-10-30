package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SignupItem is one thing the Instance asks or tells a person at sign-up (ADR 0062): a required
// acceptance, an optional consent or a notice, set by a Staff member. It belongs to the Instance,
// not to a Workspace, so it has no scoped wrapper; only internal/signupitems reads and writes it.
// An item is never deleted: its published Legal document versions keep it, and it is archived
// instead (DiscardMixin, glossary Discard): it then leaves the sign-up form and its public page,
// and is never restored.
type SignupItem struct {
	ent.Schema
}

func (SignupItem) Annotations() []schema.Annotation {
	a := TableWithEnumChecks("signup_items", SignupItem{}.Fields())
	// Only an optional consent can stand for consent to the operator's mailings.
	a.Checks["signup_items_mailings_consent_check"] = "NOT mailings_consent OR kind = 'optional_consent'"
	return []schema.Annotation{a}
}

func (SignupItem) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, DiscardMixin{}}
}

func (SignupItem) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The item's address: its public page is served at a fixed path of the key. Unique across
		// archived items too, not partial as DiscardMixin asks: a User's answers are matched to an
		// item by its key, so a new item that took an archived one's key would inherit its answers.
		field.String("key").
			NotEmpty().
			MaxLen(64).
			SchemaType(map[string]string{dialect.Postgres: "varchar(64)"}).
			Unique().
			Immutable(),
		field.String("title").
			NotEmpty().
			MaxLen(200).
			SchemaType(map[string]string{dialect.Postgres: "varchar(200)"}),
		field.Enum("kind").
			Values("required_acceptance", "optional_consent", "notice").
			Immutable(),
		// Where the item stands in the sign-up form, smallest first.
		field.Int("position"),
		// Whether the item stands for consent to the operator's mailings: a setting the Staff
		// member chooses, held by at most one item.
		field.Bool("mailings_consent").
			Default(false),
	}
}

func (SignupItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("draft", LegalDocumentDraft.Type).
			Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
		// A published version is never deleted, so an item that has one is never deleted either.
		edge.To("versions", LegalDocumentVersion.Type).
			Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (SignupItem) Indexes() []ent.Index {
	return []ent.Index{
		// At most one item is the mailings consent.
		index.Fields("mailings_consent").
			Unique().
			Annotations(entsql.IndexWhere("mailings_consent")),
	}
}
