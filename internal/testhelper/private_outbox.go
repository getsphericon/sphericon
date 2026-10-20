package testhelper

import (
	watermillsql "github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"

	"github.com/getsphericon/sphericon/internal/events"
)

// PrivateOutbox makes a subscriber read the outbox of a database of the test's own
// (RealDB) without waiting for transactions it cannot see.
//
// watermill's default adapter delivers a row only when its transaction id is below
// pg_snapshot_xmin, and that horizon is cluster-wide: any open transaction in any
// database of the shared test server (another package's rolled-back test, another
// checkout's dev backend) holds every subscriber back until it ends, so delivery
// takes as long as the longest unrelated transaction. The horizon exists so a row
// that commits late is not overtaken by a later one; in a database no one else writes
// to, the only writer is the test, which commits before the subscriber can read, so
// nothing can be overtaken. Never use it on a database other writers share.
func PrivateOutbox() events.SubscribeOption {
	return events.WithSchemaAdapter(privateSchema{})
}

// privateSchema is the default Postgres adapter with the horizon condition left out of
// the select query, everything else (table layout, ordering, the default batch of 100) unchanged.
type privateSchema struct {
	watermillsql.DefaultPostgreSQLSchema
}

func (s privateSchema) SelectQuery(params watermillsql.SelectQueryParams) (watermillsql.Query, error) {
	nextOffset, err := params.OffsetsAdapter.NextOffsetQuery(watermillsql.NextOffsetQueryParams{
		Topic:         params.Topic,
		ConsumerGroup: params.ConsumerGroup,
	})
	if err != nil {
		return watermillsql.Query{}, err
	}
	query := `
	WITH last_processed AS (
		` + nextOffset.Query + `
	)
	SELECT "offset", transaction_id, uuid, payload, metadata FROM ` + s.MessagesTable(params.Topic) + `
	WHERE
		(transaction_id = (SELECT last_processed_transaction_id FROM last_processed)
			AND "offset" > (SELECT offset_acked FROM last_processed))
		OR transaction_id > (SELECT last_processed_transaction_id FROM last_processed)
	ORDER BY transaction_id ASC, "offset" ASC
	LIMIT 100`
	return watermillsql.Query{Query: query, Args: nextOffset.Args}, nil
}
