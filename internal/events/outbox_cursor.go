package events

import (
	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent/outboxmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
)

// The subscribers read the outbox in (transaction_id, offset) order, and a consumer
// group's cursor is that pair. Comparing a row with a cursor is a row-value comparison,
// which ent has no predicate for, so it is written once here, on the generated query,
// for both the lag gauge and the prune.

// outboxAfter selects the rows that sort strictly after the cursor: what the group
// has not consumed.
func outboxAfter(transactionID string, offset int64) predicate.OutboxMessage {
	return outboxCompare(">", transactionID, offset)
}

// outboxBefore selects the rows that sort strictly before the cursor: what every group
// past it has consumed.
func outboxBefore(transactionID string, offset int64) predicate.OutboxMessage {
	return outboxCompare("<", transactionID, offset)
}

func outboxCompare(op, transactionID string, offset int64) predicate.OutboxMessage {
	return predicate.OutboxMessage(func(s *sql.Selector) {
		// raw sql: a row-value comparison (transaction_id, id) against an xid8 cursor; ent has no
		// tuple comparison or xid8 cast.
		s.Where(sql.P(func(b *sql.Builder) {
			b.WriteString("(").
				WriteString(s.C(outboxmessage.FieldTransactionID)).WriteString(", ").
				WriteString(s.C(outboxmessage.FieldID)).
				WriteString(") " + op + " (").
				Arg(transactionID).WriteString("::xid8, ").
				Arg(offset).WriteString("::bigint)")
		}))
	})
}
