package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// IdentitySecret is the secret a Workspace's server signs Identity tokens with (GLOSSARY "Identity
// secret", ADR 0171): at most one row per Workspace, made when an Owner or Admin first generates it
// and replaced by each regeneration. It is stored sealed, as a webhook's signing secret is.
type IdentitySecret struct {
	ent.Schema
}

func (IdentitySecret) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "identity_secrets"},
		Audited{Action: "identity_secret"},
	}
}

func (IdentitySecret) Mixin() []ent.Mixin {
	// No revision: nobody edits it, a regeneration replaces it whatever was read.
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "identity_secrets"}}
}

func (IdentitySecret) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The HS256 key, encrypted at rest via internal/kernel/secrets.Cipher.
		field.String("secret_encrypted").
			Sensitive().
			Annotations(Sensitive{}),
	}
}

func (IdentitySecret) Indexes() []ent.Index {
	return []ent.Index{
		// One Identity secret per Workspace.
		index.Fields("workspace_id").Unique(),
	}
}
