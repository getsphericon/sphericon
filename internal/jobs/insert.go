package jobs

import (
	"context"
	"database/sql"
	"slices"

	"github.com/riverqueue/river"
)

// insertChunk bounds a single InsertMany so a very large fan-out is queued in pieces rather than
// one huge statement.
const insertChunk = 1000

// insertMany queues params through rc, insertChunk at a time. River refuses an empty InsertMany,
// so no params queue nothing.
func insertMany(ctx context.Context, rc *river.Client[*sql.Tx], params []river.InsertManyParams) error {
	for chunk := range slices.Chunk(params, insertChunk) {
		if _, err := rc.InsertMany(ctx, chunk); err != nil {
			return err
		}
	}
	return nil
}

// queueEach fans a job out into one job per row, args(row) each, through the river client of the
// job it runs in: the shape of every periodic tick that queues a check per due row, so each row
// gets its own isolation and retry. No rows queue nothing and need no river client.
func queueEach[T any](ctx context.Context, rows []T, args func(T) river.JobArgs) error {
	if len(rows) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, len(rows))
	for i, row := range rows {
		params[i] = river.InsertManyParams{Args: args(row)}
	}
	return insertMany(ctx, river.ClientFromContext[*sql.Tx](ctx), params)
}
