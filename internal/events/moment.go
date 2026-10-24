package events

import (
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent/event"
)

// Moment is the SQL expression of when an Event happened (ADR 0044): `occurred_at`, and the
// creation time when it has none, so backfilled history lands on the day it happened. Every
// report and window reads an Event's time through it.
func Moment(sel *sql.Selector) string {
	// raw sql: ent has no COALESCE operand for a predicate, a selected column or a range.
	return "COALESCE(" + sel.C(event.FieldOccurredAt) + ", " + sel.C(event.FieldCreatedAt) + ")"
}

// MomentBetween narrows a query of Events (through Modify) to the ones that happened in
// [since, until): the start is inclusive, the end exclusive, both bound as arguments.
func MomentBetween(sel *sql.Selector, since, until time.Time) {
	moment := Moment(sel)
	// raw sql: a range over the Moment expression, which ent's predicates cannot take as an operand.
	sel.Where(sql.P(func(b *sql.Builder) {
		b.WriteString(moment).WriteString(" >= ").Arg(since).
			WriteString(" AND ").WriteString(moment).WriteString(" < ").Arg(until)
	}))
}
