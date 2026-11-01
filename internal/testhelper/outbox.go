package testhelper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
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

// OutboxEnvelopes returns the envelopes published on the outbox in this test's
// transaction, oldest first. With names, only envelopes of those event names.
func (env *TestEnv) OutboxEnvelopes(t *testing.T, names ...string) []events.Envelope {
	t.Helper()
	envelopes, err := readOutbox(t.Context(), env.DB, names)
	require.NoError(t, err)
	return envelopes
}

// Publisher is a Publisher onto this test's outbox, for a test that calls a function taking the
// Publisher of a transaction it owns (automations.Enroll) without opening one: each event is
// published in a bus transaction of its own, which rides the test's.
func (env *TestEnv) Publisher() events.Publisher { return PublisherOn(env.Bus) }

// PublisherOn is Publisher for a test that holds a bus of its own (one over a real database).
func PublisherOn(bus *events.Bus) events.Publisher { return busPublisher{bus: bus} }

type busPublisher struct{ bus *events.Bus }

func (p busPublisher) Publish(ctx context.Context, ev events.DomainEvent) error {
	return p.bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error { return pub.Publish(ctx, ev) })
}

// DeliverToEE hands every envelope on this test's outbox to the Enterprise
// subscribers, as the router would (no router runs under txdb). Each call redelivers
// the whole outbox, so calling it twice exercises at-least-once redelivery.
func (env *TestEnv) DeliverToEE(t *testing.T) {
	t.Helper()
	envelopes := env.OutboxEnvelopes(t)
	for i := range envelopes {
		e := &envelopes[i]
		for _, c := range env.edition.Consumers {
			require.NoError(t, c.Handle(t.Context(), *e), c.Name)
		}
	}
}

// DeliverOrders hands every envelope on this test's outbox to the orders subscriber, as the
// router would (no router runs under txdb). Each call redelivers the whole outbox, so calling it
// twice exercises at-least-once redelivery.
func (env *TestEnv) DeliverOrders(t *testing.T) {
	t.Helper()
	envelopes := env.OutboxEnvelopes(t)
	for i := range envelopes {
		require.NoError(t, events.RecordOrder(t.Context(), env.DB, env.Orders, envelopes[i]))
	}
}

// OutboxEvents is OutboxEnvelopes decoded into their typed events.
func (env *TestEnv) OutboxEvents(t *testing.T, names ...string) []events.DomainEvent {
	t.Helper()
	envelopes := env.OutboxEnvelopes(t, names...)
	out := make([]events.DomainEvent, 0, len(envelopes))
	for i := range envelopes {
		ev, err := events.Decode(envelopes[i])
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
	envelopes := env.OutboxEnvelopes(t, names...)
	out := make([]OutboxMessage, 0, len(envelopes))
	for i := range envelopes {
		e := &envelopes[i]
		dec := json.NewDecoder(bytes.NewReader(e.Data))
		dec.UseNumber()
		var data map[string]any
		require.NoError(t, dec.Decode(&data))
		out = append(out, OutboxMessage{Envelope: *e, Data: data})
	}
	return out
}

// OutboxCount counts the outbox messages named name whose payload has every
// key/value of match (values compare by their printed form: 42 matches "42").
func (env *TestEnv) OutboxCount(t *testing.T, name string, match map[string]any) int {
	t.Helper()
	n := 0
	messages := env.Outbox(t, name)
	for i := range messages {
		m := &messages[i]
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

// OutboxEnvelopesOn is OutboxEnvelopes for a database a test wrote to for real, read
// through the ent client the test holds on it.
func OutboxEnvelopesOn(ctx context.Context, client *ent.Client, names ...string) ([]events.Envelope, error) {
	return readOutbox(ctx, client, names)
}

func readOutbox(ctx context.Context, client *ent.Client, names []string) ([]events.Envelope, error) {
	messages, err := client.OutboxMessage.Query().Order(outboxmessage.ByID()).All(ctx)
	if err != nil {
		return nil, err
	}
	var out []events.Envelope
	for _, m := range messages {
		var metadata map[string]string
		if len(m.Metadata) > 0 {
			if err := json.Unmarshal(m.Metadata, &metadata); err != nil {
				return nil, err
			}
		}
		e, err := events.EnvelopeFrom(m.UUID, m.Payload, metadata)
		if err != nil {
			return nil, err
		}
		if len(names) == 0 || slices.Contains(names, e.Name) {
			out = append(out, e)
		}
	}
	return out, nil
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
