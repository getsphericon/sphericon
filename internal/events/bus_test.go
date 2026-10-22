package events_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func outboxCount(t *testing.T, env *testhelper.TestEnv) int {
	t.Helper()
	return len(env.OutboxEnvelopes(t))
}

// dataFor marshals a typed event into the envelope Data, as the publisher does.
func dataFor(t *testing.T, ev events.DomainEvent) []byte {
	t.Helper()
	b, err := json.Marshal(ev)
	require.NoError(t, err)
	return b
}

// The load-bearing requirement: the state row and the outbox row commit together.
func TestBusWithinTxCommitsStateAndOutbox(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	before := outboxCount(t, env)

	var created *ent.Contact
	err := env.Bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		c, err := tx.Contact.Create().
			SetWorkspaceID(fixtures.AcmeID).
			SetEmail("outbox-commit@example.com").
			Save(ctx)
		if err != nil {
			return err
		}
		created = c
		return pub.Publish(ctx, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: c.ID, Email: *c.Email})
	})
	require.NoError(t, err)

	// State row committed (visible through the shared txdb connection)...
	got, err := env.DB.Contact.Query().Where(contact.Email("outbox-commit@example.com")).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	// ...and exactly one outbox row was appended in the same transaction.
	assert.Equal(t, before+1, outboxCount(t, env))
}

// If the unit of work fails, neither the state nor the event is published.
func TestBusWithinTxRollsBackBoth(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	before := outboxCount(t, env)

	boom := errors.New("boom")
	err := env.Bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		if _, err := tx.Contact.Create().
			SetWorkspaceID(fixtures.AcmeID).
			SetEmail("outbox-rollback@example.com").
			Save(ctx); err != nil {
			return err
		}
		if err := pub.Publish(ctx, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, Email: "outbox-rollback@example.com"}); err != nil {
			return err
		}
		return boom
	})
	require.ErrorIs(t, err, boom)

	exists, err := env.DB.Contact.Query().Where(contact.Email("outbox-rollback@example.com")).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists, "contact must not be committed when the unit of work fails")
	assert.Equal(t, before, outboxCount(t, env), "no event published on rollback")
}

// Persist projects an envelope to the durable Event row. This is the load-bearing
// check on the projection (the router doesn't run under txdb): every column —
// including the resolved contact_id, visitor_id, phone, a non-now OccurredAt, and
// opaque Properties — must land. Uses a CollectedEvent because it exercises them all.
func TestPersistWritesEventProjection(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	occurred := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	collected := &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID,
		ContactID:   fixtures.ContactAliceID,
		VisitorID:   "visitor-abc",
		SubjectID:   "visitor:abc",
		Action:      "page_view",
		Email:       "persist@example.com",
		Phone:       "+15550100",
		Properties:  map[string]any{"path": "/pricing"},
		OccurredAt:  occurred,
	}
	envlp := events.Envelope{
		ID:          "01J0PERSISTTEST00000000000",
		Name:        events.NameCollected,
		Version:     1,
		WorkspaceID: fixtures.AcmeID,
		OccurredAt:  occurred,
		Data:        dataFor(t, collected),
	}
	require.NoError(t, events.Persist(ctx, env.DB, envlp))

	row, err := env.DB.Event.Query().Where(event.SourceID(envlp.ID)).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, "page_view", row.Action)
	assert.Equal(t, "visitor:abc", row.SubjectID)
	require.NotNil(t, row.Email)
	assert.Equal(t, "persist@example.com", *row.Email)
	require.NotNil(t, row.Phone)
	assert.Equal(t, "+15550100", *row.Phone)
	require.NotNil(t, row.ContactID)
	assert.EqualValues(t, fixtures.ContactAliceID, *row.ContactID)
	require.NotNil(t, row.VisitorID)
	assert.Equal(t, "visitor-abc", *row.VisitorID)
	require.NotNil(t, row.OccurredAt)
	assert.Equal(t, occurred, row.OccurredAt.UTC())
	assert.Equal(t, "/pricing", row.Properties["path"])
}

// An Event outlives its Contact (GLOSSARY: Event). Delivery is asynchronous, so the
// Contact can be gone, or never have been this Workspace's, when the event is persisted:
// the row is written unlinked instead of failing on the contact_id foreign key.
func TestPersistLeavesAnEventUnlinkedWhenItsContactIsGone(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	for id, contactID := range map[string]int64{
		// A Contact that is gone: its id no longer exists.
		"01J0PERSISTGONE0000000000": 999999,
		// Globex's contact is not an Acme contact.
		"01J0PERSISTFOREIGN00000000": fixtures.ContactGlobexID,
	} {
		collected := &events.CollectedEvent{WorkspaceID: fixtures.AcmeID, ContactID: contactID, Action: "page_view"}
		require.NoError(t, events.Persist(ctx, env.DB, events.Envelope{
			ID: id, Name: events.NameCollected, Version: 1, WorkspaceID: fixtures.AcmeID,
			OccurredAt: time.Now(), Data: dataFor(t, collected),
		}))
		row, err := env.DB.Event.Query().Where(event.SourceID(id)).Only(ctx)
		require.NoError(t, err)
		assert.Nil(t, row.ContactID, id)
	}
}

// Deleting a Contact unlinks its Events and keeps them.
func TestDeletingAContactKeepsItsEvents(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	ev, err := env.DB.Event.Query().Where(event.ContactID(fixtures.ContactAliceID)).First(ctx)
	require.NoError(t, err, "the fixtures hold Events of Alice")

	require.NoError(t, env.DB.Contact.DeleteOneID(fixtures.ContactAliceID).Exec(ctx))

	kept, err := env.DB.Event.Get(ctx, ev.ID)
	require.NoError(t, err)
	assert.Nil(t, kept.ContactID)
}

// At-least-once delivery can redeliver an envelope; persist must dedupe on the
// source id (envelope ULID) so the projection row is written at most once.
func TestPersistIsIdempotent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	envlp := events.Envelope{
		ID:          "01J0IDEMPOTENT00000000000",
		Name:        events.NameContactCreated,
		Version:     1,
		WorkspaceID: fixtures.AcmeID,
		OccurredAt:  time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC),
		Data:        dataFor(t, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: 7, Email: "dedupe@example.com"}),
	}
	require.NoError(t, events.Persist(ctx, env.DB, envlp))
	require.NoError(t, events.Persist(ctx, env.DB, envlp)) // redelivery

	n, err := env.DB.Event.Query().Where(event.SourceID(envlp.ID)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "redelivered envelope must not write a second row")
}
