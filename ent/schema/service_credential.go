package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// ServiceCredential is a machine identity on the Operator surface (ADR 0026): a bearer
// credential a service presents on the internal listener, limited to named scopes. Like
// Operator it belongs to no Workspace, so the scoped client does not cover it; it is
// created, rotated and revoked only by the CLI and is read and written only from
// ee/operator. Only a hash of its value is stored. A service may hold several active
// credentials at once (the name is not unique), so a rotation needs no downtime.
type ServiceCredential struct {
	ent.Schema
}

func (ServiceCredential) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "service_credentials"},
	}
}

func (ServiceCredential) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (ServiceCredential) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The service's name, as recorded in the Audit log. Not unique: two active
		// credentials of one service share it.
		field.String("name").
			NotEmpty().
			Immutable(),
		// The visible, unique part of the value, which finds the row; the secret part
		// is verified against secret_hash.
		field.String("prefix").
			NotEmpty().
			Unique().
			Immutable(),
		field.String("secret_hash").
			NotEmpty().
			Sensitive(),
		field.JSON("scopes", []string{}).
			Default([]string{}),
		field.Time("expires_at").
			Optional().
			Nillable(),
		field.Time("revoked_at").
			Optional().
			Nillable(),
		field.Time("last_used_at").
			Optional().
			Nillable(),
	}
}
