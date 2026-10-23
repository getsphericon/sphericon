package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Passkey is a WebAuthn credential a User registered to log in without a password
// (ADR 0052). It belongs to the User, not a Workspace, and only internal/secondfactor
// reads and writes it. The columns are what go-webauthn needs to rebuild a credential
// and check an assertion against it; the backup flags are stored as registered because
// the library refuses an assertion whose Backup Eligible flag differs.
type Passkey struct {
	ent.Schema
}

func (Passkey) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "passkeys"},
	}
}

func (Passkey) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (Passkey) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("user_id").
			Immutable(),
		// The authenticator's credential id: how a usernameless login finds the User.
		field.Bytes("credential_id").
			NotEmpty().
			Unique().
			Immutable(),
		// The COSE public key.
		field.Bytes("public_key").
			NotEmpty().
			Immutable(),
		field.Int64("sign_count").
			Default(0),
		field.JSON("transports", []string{}).
			Optional(),
		field.Bool("backup_eligible").
			Immutable(),
		field.Bool("backup_state"),
		field.Bytes("aaguid").
			Optional().
			Immutable(),
		field.String("attestation_type").
			Optional().
			Immutable(),
		// What the User calls it, to tell their Passkeys apart.
		field.String(nameField).
			NotEmpty(),
		field.Time("last_used_at").
			Optional().
			Nillable(),
	}
}

func (Passkey) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
	}
}

func (Passkey) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("passkeys").
			Field("user_id").
			Unique().
			Required().
			Immutable(),
	}
}
