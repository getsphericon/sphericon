package testhelper

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/getsphericon/sphericon/ee/abuse"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/events"
)

// The domain-event outbox is a watermill table, not an ent entity, so tests read it
// through these helpers instead of querying it themselves.

// OutboxEnvelopes returns the envelopes published on the outbox in this test's
// transaction, oldest first. With names, only envelopes of those event names.
func (env *TestEnv) OutboxEnvelopes(t *testing.T, names ...string) []events.Envelope {
	t.Helper()
	envelopes, err := readOutbox(t.Context(), env.SQLDB, names)
	require.NoError(t, err)
	return envelopes
}

// DeliverToEE hands every envelope on this test's outbox to the Enterprise
// subscribers, as the router would (no router runs under txdb). Each call redelivers
// the whole outbox, so calling it twice exercises at-least-once redelivery.
func (env *TestEnv) DeliverToEE(t *testing.T) {
	t.Helper()
	for _, e := range env.OutboxEnvelopes(t) {
		for _, c := range env.edition.Consumers {
			require.NoError(t, c.Handle(t.Context(), e), c.Name)
		}
	}
}

// OutboxEvents is OutboxEnvelopes decoded into their typed events.
func (env *TestEnv) OutboxEvents(t *testing.T, names ...string) []events.DomainEvent {
	t.Helper()
	var out []events.DomainEvent
	for _, e := range env.OutboxEnvelopes(t, names...) {
		ev, err := events.Decode(e)
		require.NoError(t, err)
		out = append(out, ev)
	}
	return out
}

// OutboxMessage is one outbox envelope with its payload decoded generically
// (numbers as json.Number, so ids compare exactly).
type OutboxMessage struct {
	Envelope events.Envelope
	Data     map[string]any
}

// Outbox returns the outbox messages of this test's transaction, oldest first,
// optionally only those of the given event names.
func (env *TestEnv) Outbox(t *testing.T, names ...string) []OutboxMessage {
	t.Helper()
	var out []OutboxMessage
	for _, e := range env.OutboxEnvelopes(t, names...) {
		dec := json.NewDecoder(bytes.NewReader(e.Data))
		dec.UseNumber()
		var data map[string]any
		require.NoError(t, dec.Decode(&data))
		out = append(out, OutboxMessage{Envelope: e, Data: data})
	}
	return out
}

// OutboxCount counts the outbox messages named name whose payload has every
// key/value of match (values compare by their printed form: 42 matches "42").
func (env *TestEnv) OutboxCount(t *testing.T, name string, match map[string]any) int {
	t.Helper()
	n := 0
	for _, m := range env.Outbox(t, name) {
		ok := true
		for k, want := range match {
			if fmt.Sprint(m.Data[k]) != fmt.Sprint(want) {
				ok = false
				break
			}
		}
		if ok {
			n++
		}
	}
	return n
}

// PurgeOutbox empties the outbox of a database a test wrote to for real (outside
// the per-test transaction), such as the router tests.
func PurgeOutbox(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `DELETE FROM watermill_domain_events`)
	return err
}

// OutboxEnvelopesOn is OutboxEnvelopes for a database a test wrote to for real.
func OutboxEnvelopesOn(ctx context.Context, db *sql.DB, names ...string) ([]events.Envelope, error) {
	return readOutbox(ctx, db, names)
}

func readOutbox(ctx context.Context, db *sql.DB, names []string) ([]events.Envelope, error) {
	rows, err := db.QueryContext(ctx, `SELECT payload FROM watermill_domain_events ORDER BY "offset"`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []events.Envelope
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var e events.Envelope
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, err
		}
		if len(names) == 0 || slices.Contains(names, e.Name) {
			out = append(out, e)
		}
	}
	return out, rows.Err()
}

// OutboxRow is one raw outbox row: its position in the (transaction_id, offset)
// order the subscribers read in, and when it was written.
type OutboxRow struct {
	TxID      uint64
	Offset    int64
	CreatedAt time.Time
}

// OutboxRows returns every outbox row of this test's transaction in
// (transaction_id, offset) order.
func (env *TestEnv) OutboxRows(t *testing.T) []OutboxRow {
	t.Helper()
	rows, err := env.SQLDB.QueryContext(t.Context(),
		`SELECT transaction_id::text::bigint, "offset", created_at FROM watermill_domain_events ORDER BY transaction_id, "offset"`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var out []OutboxRow
	for rows.Next() {
		var r OutboxRow
		var tx int64
		require.NoError(t, rows.Scan(&tx, &r.Offset, &r.CreatedAt))
		r.TxID = uint64(tx)
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

// AgeOutbox backdates the outbox rows with the given offsets by age (all rows when
// no offsets are given), so a test can place rows on either side of a retention
// floor; the producer always stamps rows with the current time.
func (env *TestEnv) AgeOutbox(t *testing.T, age time.Duration, offsets ...int64) {
	t.Helper()
	q := `UPDATE watermill_domain_events SET created_at = created_at - make_interval(secs => $1)`
	args := []any{age.Seconds()}
	if len(offsets) > 0 {
		q += ` WHERE "offset" = ANY($2)`
		args = append(args, offsets)
	}
	_, err := env.SQLDB.ExecContext(t.Context(), q, args...)
	require.NoError(t, err)
}

// OutboxCursor is one consumer group's stored position. Acked is nil for a
// group that has never acknowledged a message.
type OutboxCursor struct {
	TxID  uint64
	Acked *int64
}

// SetOutboxCursor upserts a consumer group's offsets row, as if that group had
// acknowledged the message at offset acked of transaction txID (nil acked leaves
// the ack NULL).
func (env *TestEnv) SetOutboxCursor(t *testing.T, group string, txID uint64, acked *int64) {
	t.Helper()
	_, err := env.SQLDB.ExecContext(t.Context(), `
		INSERT INTO watermill_offsets_domain_events (consumer_group, offset_acked, last_processed_transaction_id)
		VALUES ($1, $2, $3::text::xid8)
		ON CONFLICT (consumer_group) DO UPDATE
		SET offset_acked = EXCLUDED.offset_acked, last_processed_transaction_id = EXCLUDED.last_processed_transaction_id`,
		group, acked, strconv.FormatUint(txID, 10))
	require.NoError(t, err)
}

// OutboxCursors returns every consumer group's offsets row, keyed by group.
func (env *TestEnv) OutboxCursors(t *testing.T) map[string]OutboxCursor {
	t.Helper()
	rows, err := env.SQLDB.QueryContext(t.Context(),
		`SELECT consumer_group, offset_acked, last_processed_transaction_id::text::bigint FROM watermill_offsets_domain_events`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	out := map[string]OutboxCursor{}
	for rows.Next() {
		var group string
		var acked sql.NullInt64
		var tx int64
		require.NoError(t, rows.Scan(&group, &acked, &tx))
		c := OutboxCursor{TxID: uint64(tx)}
		if acked.Valid {
			c.Acked = &acked.Int64
		}
		out[group] = c
	}
	require.NoError(t, rows.Err())
	return out
}

// ClearOutboxCursors removes every consumer group's offsets row in this test's
// transaction, so a test starts from a known state whatever real-database tests
// (the router tests) left committed.
func (env *TestEnv) ClearOutboxCursors(t *testing.T) {
	t.Helper()
	_, err := env.SQLDB.ExecContext(t.Context(), `DELETE FROM watermill_offsets_domain_events`)
	require.NoError(t, err)
}

// Abuse is the abuse detector as the composition root builds it (ADR 0026), over the
// env's bus and platform sender; a pass is Run(ctx, now).
func (env *TestEnv) Abuse() *abuse.Detector {
	return env.edition.Abuse(env.Bus, env.SystemMail)
}
