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

type Contact struct {
	ent.Schema
}

func (Contact) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("contacts", Contact{}.Fields()),
		Audited{Action: "contact", NamesOnly: true},
	}
}

func (Contact) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "contacts"}}
}

func (Contact) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// Alias keys: subject_id (the customer's own user id), email, phone. Each is
		// unique per workspace and ANY may be absent — an anonymous Contact has none.
		// Identity is multi-key; the stable anchor is the internal id, not the email.
		field.String("subject_id").
			Optional().
			Nillable(),
		field.String("email").
			Optional().
			Nillable(),
		field.String("phone").
			Optional().
			Nillable(),
		field.String("first_name").
			Optional().
			Nillable(),
		field.String("last_name").
			Optional().
			Nillable(),
		field.String("time_zone").
			Optional().
			Nillable(),
		// The ISO 3166-1 alpha-2 code of the Contact's country, upper case.
		field.String("country").
			Optional().
			Nillable(),
		// The Contact locale: a canonical BCP 47 tag (`de`, `pt-BR`).
		field.String("locale").
			Optional().
			Nillable(),
		// The date of birth, a calendar date with no time or zone.
		field.Time("birthday").
			SchemaType(map[string]string{dialect.Postgres: "date"}).
			Optional().
			Nillable(),
		field.String("city").
			Optional().
			Nillable(),
		// The https URL of the Contact's photo.
		field.String("avatar").
			Optional().
			Nillable(),
		// How the Contact first came to exist (GLOSSARY: Contact source). Set once at
		// creation and never rewritten; null for a Contact that predates the field.
		field.Enum("source").
			Values("manual", "api", "tracker").
			Optional().
			Nillable().
			Immutable(),
		// Typed Custom field values keyed by the field's machine key. The typed
		// definitions live in CustomField; values are widened to any to carry
		// number/bool/datetime, not only strings.
		field.JSON("custom_fields", map[string]any{}).
			Optional(),
	}
}

func (Contact) Edges() []ent.Edge {
	return []ent.Edge{
		// The anonymous devices seen as this person. A Visitor may belong to no
		// Contact before Identify; once bound, one Contact owns many Visitors.
		edge.To("visitors", Visitor.Type),
		// Presence-only labels (GLOSSARY: Tag), many-to-many via contact_tags.
		edge.To("tags", Tag.Type).
			Through("contact_tags", ContactTag.Type),
		// The Contact's Events. The FK is nullable and SET NULL: an Event outlives its
		// Contact (GLOSSARY: Event), so deleting the Contact only unlinks them.
		edge.To("events", Event.Type).
			Annotations(entsql.OnDelete(entsql.SetNull)),
		// The Contact's Orders (ADR 0064): unlinked like its Events, so an Order and its
		// amounts outlive the Contact.
		edge.To("orders", Order.Type).
			Annotations(entsql.OnDelete(entsql.SetNull)),
	}
}

func (Contact) Indexes() []ent.Index {
	return []ent.Index{
		// Each alias key is unique per workspace; all three are nullable and Postgres
		// treats nulls as distinct, so any may be absent without colliding.
		index.Fields("email", "workspace_id").Unique().StorageKey("contacts_email_workspace_id"),
		index.Fields("subject_id", "workspace_id").Unique().StorageKey("contacts_subject_id_workspace_id"),
		index.Fields("phone", "workspace_id").Unique().StorageKey("contacts_phone_workspace_id"),
		// The identified Contacts of a Workspace: the usage peak counts them on every
		// mutation that can move the count, and this keeps that an index-only scan of the
		// Workspace's own rows instead of a scan of the whole table.
		// Backs the Contacts Export stream (ADR 0045): the keyset page over (updated_at, id)
		// inside one Workspace.
		index.Fields("workspace_id", "updated_at", "id"),
		index.Fields("workspace_id").
			StorageKey("contacts_workspace_id_identified").
			Annotations(entsql.IndexWhere("email IS NOT NULL AND email <> ''")),
	}
}
