package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// JoinDomain is a Workspace-scoped, DNS-verified domain whose Users may join the
// Workspace automatically. An Owner adds the domain and publishes the token
// as a TXT record at _sphericon-join.<domain>; the domain is verified while the record is
// published. Any Workspace may hold a pending claim of a domain, and only one holds it
// verified (the partial unique index below).
type JoinDomain struct {
	ent.Schema
}

func (JoinDomain) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("join_domains", JoinDomain{}.Fields()),
		Audited{Action: "join_domain", NameField: "domain"},
	}
}

func (JoinDomain) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "join_domains"}}
}

func (JoinDomain) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The canonical domain: lower case, IDN as punycode, e.g. "acme.com".
		field.String("domain").
			NotEmpty(),
		// The random value the TXT record at _sphericon-join.<domain> must carry. It is
		// shown to the Owners, not a secret, and stays out of the Audit log.
		field.String("token").
			NotEmpty().
			Annotations(Sensitive{}),
		// When the domain most recently became verified (nil = pending).
		field.Time("verified_at").
			Optional().
			Nillable(),
		// When the TXT record was most recently checked (nil = never checked yet).
		field.Time("last_checked_at").
			Optional().
			Nillable(),
	}
}

func (JoinDomain) Indexes() []ent.Index {
	return []ent.Index{
		// One row per (workspace, domain).
		index.Fields("workspace_id", "domain").
			Unique(),
		// A domain is verified by one Workspace across the Instance; pending claims never
		// block anyone. The migration builds it CONCURRENTLY (ADR 0033).
		index.Fields("domain").
			Unique().
			Annotations(entsql.IndexWhere("verified_at IS NOT NULL")),
		// Lets the periodic re-check job (a later ticket) find the least-recently-checked rows.
		index.Fields("last_checked_at"),
	}
}
