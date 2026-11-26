package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
)

// ContactDate is one date a Contact holds, by its field and its key (datetrigger.Keyed, ADR 0126):
// its birthday, or a Custom field value that is a date. The date scheduler narrows the Contacts of
// a date Trigger to those whose key falls in the band of its window, on the indexes below, and
// the local calendar computes each one's moment from the Contact itself. The key is the date read
// in UTC, which no Workspace time zone moves, so a zone change rewrites no row. The contacts
// module writes the rows with the Contact, in the same transaction, from the values it writes;
// they die with it.
type ContactDate struct {
	ent.Schema
}

func (ContactDate) Annotations() []schema.Annotation {
	table := TableWithEnumChecks("contact_dates", ContactDate{}.Fields())
	// The birthday has no key; a Custom field is named by its own.
	table.Checks["contact_dates_key_check"] = "(kind = 'birthday') = (key = '')"
	// The month and day are the date's (localcalendar.Date.MonthDay).
	table.Checks["contact_dates_month_day_check"] = "month_day = EXTRACT(MONTH FROM date) * 100 + EXTRACT(DAY FROM date)"
	return []schema.Annotation{table}
}

func (ContactDate) Mixin() []ent.Mixin {
	return []ent.Mixin{WorkspaceMixin{Ref: "contact_dates"}}
}

func (ContactDate) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("contact_id").
			Immutable(),
		// The field: the birthday or a Custom field (datetrigger.FieldKind).
		field.Enum("kind").
			GoType(datetrigger.FieldKind("")).
			Immutable(),
		// The Custom field's key; empty for the birthday (contact_dates_key_check).
		field.String("key").
			Immutable(),
		// The key: the birthday, or the value's date read in UTC (datetrigger.Field.KeyOf). A once
		// date's band is a range of it.
		field.Time("date").
			SchemaType(map[string]string{dialect.Postgres: "date"}),
		// The key's month and day, month × 100 + day (contact_dates_month_day_check): a yearly date's
		// band is a set of them.
		field.Int16("month_day"),
	}
}

func (ContactDate) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("contact", Contact.Type).
			Ref("dates").
			Field("contact_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (ContactDate) Indexes() []ent.Index {
	return []ent.Index{
		// One row per Contact and field; the band read of a yearly date wider than a year pages the
		// field's holders by Contact on it.
		index.Fields("workspace_id", "kind", "key", "contact_id").
			Unique().
			StorageKey("contact_dates_field_contact"),
		// The Contact's composite foreign key, which deletes its rows with it.
		index.Fields("workspace_id", "contact_id"),
		// The band of a yearly date: a set of month and day.
		index.Fields("workspace_id", "kind", "key", "month_day"),
		// The band of a once date: a range of dates.
		index.Fields("workspace_id", "kind", "key", "date"),
	}
}
