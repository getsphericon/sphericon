package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
)

// VersionedMixin makes an entity a Versioned definition (ADR 0065, GLOSSARY: Versioned definition):
// its definition lives in versions of its own type (Version, which embeds VersionMixin), and the
// owner row keeps the two pointers into them and the state Publish moves. The live version is
// what the object does, the draft the unpublished one above it; only internal/drafts writes the
// pointers, through the operations ent/template/scoped.tmpl generates for every entity that has
// live_version_id (the scoped client's <Owner>Pointers).
type VersionedMixin struct {
	mixin.Schema

	// Version is the version entity's type (AutomationVersion.Type).
	Version any
}

// Fields are the pointers and the state. An owner passes them to TableWithEnumChecks too, so
// status is checked like the owner's own enums.
func (VersionedMixin) Fields() []ent.Field {
	return []ent.Field{
		// active: acts (enrolls Contacts, fires); inactive: acts on nothing. Publish makes the
		// object active, Deactivate and Archive inactive (ADR 0065).
		field.Enum("status").
			Values("active", "inactive").
			Default("inactive"),
		// When the object last moved from inactive to active by Publish; kept while it is
		// inactive. A Rule acts only on Events recorded at or after it, so publishing never
		// replays history.
		field.Time("activated_at").
			Optional().
			Nillable(),
		// What the object does: its live version. Nil until it is first published.
		field.Int64("live_version_id").
			Optional().
			Nillable(),
		// The unpublished changes: the one version above live, rewritten in place until it is
		// published or reverted. Nil when there are none.
		field.Int64("draft_version_id").
			Optional().
			Nillable(),
	}
}

func (m VersionedMixin) Edges() []ent.Edge {
	return []ent.Edge{
		// Purging the object (only one with no history, ADR 0049) purges its versions with it.
		edge.To("versions", m.Version).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		// The two pointers into the object's own versions. SET NULL breaks the circle of keys
		// between the object and its versions: a purge cascades to the versions, Workspace
		// erasure deletes them first.
		edge.To("live_version", m.Version).
			Field("live_version_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.To("draft_version", m.Version).
			Field("draft_version_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
	}
}

func (VersionedMixin) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("live_version_id"),
		index.Fields("draft_version_id"),
	}
}

// VersionMixin is the version side of a Versioned definition: the owner it belongs to and its
// number, unique within the owner. The body (what the version defines) stays in the version's own
// schema.
type VersionMixin struct {
	mixin.Schema

	// Owner is the owner edge's name ("automation"); the column is <Owner>_id.
	Owner string
	// OwnerType is the owner entity's type (Automation.Type).
	OwnerType any
	// Table is the version table, which names the unique index.
	Table string
}

func (m VersionMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Int64(m.Owner + "_id"),
		// 1 for the first, plus one per version after it, within one owner.
		field.Int("version").
			Positive().
			Immutable(),
	}
}

func (m VersionMixin) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From(m.Owner, m.OwnerType).
			Ref("versions").
			Field(m.Owner + "_id").
			Required().
			Unique(),
	}
}

func (m VersionMixin) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", m.Owner+"_id", "version").
			Unique().
			StorageKey(m.Table + "_workspace_id_" + m.Owner + "_id_version"),
		index.Fields(m.Owner + "_id"),
	}
}
