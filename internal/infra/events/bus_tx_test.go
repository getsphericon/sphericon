package events_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// publishedTo counts the ContactCreated the test published for the address.
func publishedTo(t *testing.T, env *testhelper.TestEnv, email string) int {
	t.Helper()
	n := 0
	for _, ev := range env.QueuedEvents(t, events.NameContactCreated) {
		if p, _ := events.ProjectionOf(ev); p.Email == email {
			n++
		}
	}
	return n
}

// An error from the transaction body rolls back the state write AND the event.
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
	assert.Zero(t, publishedTo(t, env, "rolled-back@example.com"), "and so is the event")
}

// An update ent runs in a transaction of its own joins the bus's: it commits with the event
// when the body succeeds and rolls back with it when the body fails.
func TestWithinTxOwnsTheCommitOfAnUpdate(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	rename := func(name, email string, result error) error {
		return env.Bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
			tx.Contact.UpdateOneID(fixtures.ContactAliceID).SetFirstName(name).ExecX(ctx)
			require.NoError(t, pub.Publish(ctx, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Email: email}))
			return result
		})
	}

	require.NoError(t, rename("Committed", "committed@example.com", nil))
	boom := errors.New("producer failed")
	require.ErrorIs(t, rename("Discarded", "discarded@example.com", boom), boom)

	assert.Equal(t, "Committed", *env.DB.Contact.GetX(ctx, fixtures.ContactAliceID).FirstName)
	assert.Equal(t, 1, publishedTo(t, env, "committed@example.com"), "the event commits with the update")
	assert.Zero(t, publishedTo(t, env, "discarded@example.com"))
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
	assert.Zero(t, publishedTo(t, env, "panicked@example.com"))
}

// preCommit counts its runs and fails with err.
type preCommit struct {
	runs int
	err  error
}

func (w *preCommit) Run(context.Context) error {
	w.runs++
	return w.err
}

// Work registered under one key runs once, right before the transaction commits, however many
// writes registered it.
func TestWithinTxRunsWorkRegisteredUnderAKeyOnceBeforeCommit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	work := &preCommit{}

	err := env.Bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		for range 3 {
			_, ok := tx.BeforeCommit("key", func() ent.PreCommit { return work })
			require.True(t, ok, "the bus's transaction runs work before it commits")
		}
		assert.Zero(t, work.runs, "nothing runs before the body returns")
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, work.runs)
}

// Work that fails before the commit rolls the transaction back.
func TestWithinTxRollsBackWhenWorkBeforeCommitFails(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	boom := errors.New("work failed")

	err := env.Bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		// test writes: the rollback of this write is the subject
		tx.Event.Create().SetWorkspaceID(fixtures.AcmeID).SetSubjectID("before-commit@example.com").SetAction("x").ExecX(ctx)
		tx.BeforeCommit("key", func() ent.PreCommit { return &preCommit{err: boom} })
		return nil
	})

	require.ErrorIs(t, err, boom)
	exists, err := env.DB.Event.Query().Where(event.SubjectID("before-commit@example.com")).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists)
}
