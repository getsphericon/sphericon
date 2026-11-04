package testhelper

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/abuse"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboxmessage"
	"github.com/getsphericon/sphericon/ent/outboxoffset"
	"github.com/getsphericon/sphericon/internal/events"
)

// The domain-event outbox is a watermill table that ent only generates a client for
// (ent/schema/outbox_message.go), so tests read it through these helpers.

// Publisher is a Publisher onto this test's bus, for a test that calls a function taking the
// Publisher of a transaction it owns (automations.Enroll) without opening one: each event is
// published in a bus transaction of its own, which rides the test's.
func (env *TestEnv) Publisher() events.Publisher { return PublisherOn(env.Bus) }

// PublisherOn is Publisher for a test that holds a bus of its own (one over a real database).
func PublisherOn(bus *events.Bus) events.Publisher { return busPublisher{bus: bus} }

type busPublisher struct{ bus *events.Bus }

func (p busPublisher) Publish(ctx context.Context, ev events.DomainEvent) error {
	return p.bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error { return pub.Publish(ctx, ev) })
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
	messages, err := env.DB.OutboxMessage.Query().
		Order(outboxmessage.ByTransactionID(), outboxmessage.ByID()).All(t.Context())
	require.NoError(t, err)
	out := make([]OutboxRow, len(messages))
	for i, m := range messages {
		tx, err := strconv.ParseUint(m.TransactionID, 10, 64)
		require.NoError(t, err)
		out[i] = OutboxRow{TxID: tx, Offset: m.ID, CreatedAt: m.CreatedAt}
	}
	return out
}

// AgeOutbox backdates the outbox rows with the given offsets by age (all rows when
// no offsets are given), so a test can place rows on either side of a retention
// floor; the producer always stamps rows with the current time.
func (env *TestEnv) AgeOutbox(t *testing.T, age time.Duration, offsets ...int64) {
	t.Helper()
	q := env.DB.OutboxMessage.Query()
	if len(offsets) > 0 {
		q = q.Where(outboxmessage.IDIn(offsets...))
	}
	messages, err := q.All(t.Context())
	require.NoError(t, err)
	for _, m := range messages {
		require.NoError(t, env.DB.OutboxMessage.UpdateOneID(m.ID).SetCreatedAt(m.CreatedAt.Add(-age)).Exec(t.Context()))
	}
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
	err := env.DB.OutboxOffset.Create().
		SetID(group).
		SetNillableOffsetAcked(acked).
		SetLastProcessedTransactionID(strconv.FormatUint(txID, 10)).
		OnConflictColumns(outboxoffset.FieldID).
		Update(func(u *ent.OutboxOffsetUpsert) {
			u.UpdateLastProcessedTransactionID()
			if acked == nil {
				u.ClearOffsetAcked() // overwrite an earlier ack with NULL
			} else {
				u.UpdateOffsetAcked()
			}
		}).
		Exec(t.Context())
	require.NoError(t, err)
}

// OutboxCursors returns every consumer group's offsets row, keyed by group.
func (env *TestEnv) OutboxCursors(t *testing.T) map[string]OutboxCursor {
	t.Helper()
	offsets, err := env.DB.OutboxOffset.Query().All(t.Context())
	require.NoError(t, err)
	out := map[string]OutboxCursor{}
	for _, o := range offsets {
		tx, err := strconv.ParseUint(o.LastProcessedTransactionID, 10, 64)
		require.NoError(t, err)
		out[o.ID] = OutboxCursor{TxID: tx, Acked: o.OffsetAcked}
	}
	return out
}

// Abuse is the abuse detector as the composition root builds it (ADR 0026), over the
// env's bus and platform sender; a pass is Run(ctx, now).
func (env *TestEnv) Abuse() *abuse.Detector {
	return env.edition.Abuse(env.Bus, env.SystemMail)
}
