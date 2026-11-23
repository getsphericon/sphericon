package automations

import (
	"context"
	"database/sql"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/predicate"
)

// The Enrollment key (ADR 0165): a version may name a property of its Trigger Event, `order_id`,
// and its Enrollments are then one per value of it. An Event's value of the key is the property's
// string form as Postgres reads it (properties ->> key): a string as it is, a number, a boolean, an
// object or a list as its JSON text, and a JSON null as no value. It is read in one place, Postgres,
// so the value an Enrollment keeps and the value an exit or goal Event carries are written alike:
// a number decoded into Go would print otherwise.

// keyText writes the Event's value of the key in a statement over the events table: its
// properties' member key as text, NULL when it has none.
func keyText(sel *entsql.Selector, key string) func(b *entsql.Builder) {
	return func(b *entsql.Builder) {
		b.WriteString("(").Ident(sel.C(event.FieldProperties)).WriteString(" ->> ").Arg(key).WriteString(")")
	}
}

// keyValueOf is the value of the key the Event stored with sourceID carries, read from its whole
// stored properties (an Event weighing more than a Rule sees still has its key); nil when it
// carries none, or no such Event is stored (Retention or an Erasure removed it, or none began the
// entry).
func keyValueOf(ctx context.Context, s *ent.Scoped, sourceID, key string) (*string, error) {
	if sourceID == "" {
		return nil, nil
	}
	var rows []struct {
		Value sql.NullString `sql:"value"`
	}
	err := s.Event().Query().
		Where(event.SourceID(sourceID)).
		Modify(func(sel *entsql.Selector) {
			// raw sql: ent has no expression for a JSON member read as text (->>).
			sel.Select().AppendSelectExprAs(entsql.ExprFunc(keyText(sel, key)), "value")
		}).
		Scan(ctx, &rows)
	if err != nil || len(rows) == 0 || !rows[0].Value.Valid {
		return nil, err
	}
	return &rows[0].Value.String, nil
}

// ofKey is the predicate that a stored Event may act on the Enrollment whose key value is value: it
// carries the key with that value, or does not carry the key, and then acts on every Enrollment of
// the Contact in the Automation (ADR 0165).
func ofKey(key, value string) predicate.Event {
	return func(sel *entsql.Selector) {
		member := keyText(sel, key)
		// raw sql: ent has no expression for a JSON member read as text (->>), and its sqljson
		// comparisons compare JSON values, not the text form an Enrollment keeps.
		sel.Where(entsql.P(func(b *entsql.Builder) {
			b.WriteString("(")
			member(b)
			b.WriteString(" IS NULL OR ")
			member(b)
			b.WriteString(" = ").Arg(value).WriteString(")")
		}))
	}
}

// endsKey is whether the Event stored with sourceID may end the Enrollment: an Enrollment of a
// keyed version only when the Event carries none of the key or the Enrollment's value (ADR 0165),
// any other always. values caches the Event's value of each key name read, so one Event is read
// once per key however many Enrollments it is asked about.
func endsKey(ctx context.Context, s *ent.Scoped, run *ent.AutomationRun, sourceID string, values map[string]*string) (bool, error) {
	key := run.Edges.Version.EnrollmentKey
	if key == nil || run.KeyValue == nil {
		return true, nil
	}
	value, read := values[*key]
	if !read {
		readValue, err := keyValueOf(ctx, s, sourceID, *key)
		if err != nil {
			return false, err
		}
		value = readValue
		values[*key] = value
	}
	return value == nil || *value == *run.KeyValue, nil
}
