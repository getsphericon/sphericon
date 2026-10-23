package events_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func outboxRows(t *testing.T, env *testhelper.TestEnv, email string) int {
	t.Helper()
	n := 0
	for _, ev := range env.OutboxEvents(t, events.NameContactCreated) {
		if ev.Project().Email == email {
			n++
		}
	}
	return n
}

// An error from the transaction body rolls back the state write AND the outbox row.
func TestWithinTxRollsBackOnError(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	boom := errors.New("producer failed")

	err := env.Bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		// test writes: transaction commit and rollback through the bus is the subject
		tx.Event.Create().SetWorkspaceID(fixtures.AcmeID).SetSubjectID("rolled-back@example.com").SetAction("x").ExecX(ctx)
		require.NoError(t, pub.Publish(ctx, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: 7, Email: "rolled-back@example.com"}))
		return boom
	})
	require.ErrorIs(t, err, boom)

	exists, err := env.DB.Event.Query().Where(event.SubjectID("rolled-back@example.com")).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists, "the state write is rolled back")
	assert.Zero(t, outboxRows(t, env, "rolled-back@example.com"), "and so is the outbox row")
}

// A panic in the body rolls the transaction back and propagates to the caller.
func TestWithinTxRollsBackAndRepanics(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	assert.PanicsWithValue(t, "kaboom", func() {
		err := env.Bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
			require.NoError(t, pub.Publish(ctx, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: 8, Email: "panicked@example.com"}))
			panic("kaboom")
		})
		require.NoError(t, err) // not reached: the callback panics, and WithinTx rolls back and repanics
	})
	assert.Zero(t, outboxRows(t, env, "panicked@example.com"))
}

// Persisting a collected event carries the visitor and phone onto the projection row.
func TestPersistCollectedEventProjectsVisitorAndPhone(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	data := dataFor(t, &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID, VisitorID: "vis-1", Action: "page_view", Phone: "+15550100", Email: "vis@example.com",
	})
	require.NoError(t, events.Persist(ctx, env.DB, events.Envelope{
		ID: "01J0PERSISTCOLLECTED00000", Name: events.NameCollected, Version: 1, WorkspaceID: fixtures.AcmeID, Data: data,
	}))

	got, err := env.DB.Event.Query().Where(event.SourceID("01J0PERSISTCOLLECTED00000")).Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, got.VisitorID)
	require.NotNil(t, got.Phone)
	require.NotNil(t, got.Email)
	assert.Equal(t, "vis-1", *got.VisitorID)
	assert.Equal(t, "+15550100", *got.Phone)
	assert.Equal(t, "vis@example.com", *got.Email)
}
