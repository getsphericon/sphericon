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
// they die with it. A row is never updated: a write that gives the field another value replaces
// it, so its write time and its send lock are those of the write that set its value, and a write
// that leaves the value as it was keeps both.
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
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "contact_dates"}}
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
			SchemaType(map[string]string{dialect.Postgres: "date"}).
			Immutable(),
		// The key's month and day, month × 100 + day (contact_dates_month_day_check): a yearly date's
		// band is a set of them.
		field.Int("month_day").
			Immutable(),
		// When the write that set the field's value was made, on the Instance clock (decision 15 of #1613): an
		// occurrence of the date is `missed` only when the date was written by its moment; one
		// written after it is a date set late.
		field.Time("written_at").
			Annotations(Attribute("a fact of the write that set the value, fixed with the row; the date scheduler compares it with an occurrence's moment")).
			Immutable(),
		// The write that set the value came through a Connection without mcp:send (ADR 0132 as
		// amended by ADR 0126): an occurrence of a date Trigger on it enrolls no one. A write of
		// another value without the mark replaces the row, and lifts it.
		field.Bool("send_locked").
			Annotations(Attribute("a fact of the write that set the value, fixed with the row, which a write of another value replaces")).
			Immutable(),
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
