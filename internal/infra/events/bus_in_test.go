package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Every client a bus transaction hands out is one PublishIn and EnqueueIn take.
var (
	_ events.InTx = (*ent.Client)(nil)
	_ events.InTx = (*ent.Scoped)(nil)
	_ events.InTx = (*ent.WorkspaceLocked)(nil)
)

// publishAndEnqueueIn publishes a ContactCreated for email and queues a send job for recipient in
// the transaction in is bound to.
func publishAndEnqueueIn(ctx context.Context, bus *events.Bus, in events.InTx, email string, recipient int64) error {
	if err := bus.PublishIn(ctx, in, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Email: email}); err != nil {
		return err
	}
	return bus.EnqueueIn(ctx, in, jobs.SendRecipientArgs{WorkspaceID: fixtures.AcmeID, RecipientID: recipient, BroadcastID: 1}, nil)
}

// queuedSends counts the send jobs queued for recipient.
func queuedSends(t *testing.T, env *testhelper.TestEnv, recipient int64) int {
	t.Helper()
	n := 0
	for _, args := range env.JobsOf(t, jobkind.SendRecipient.Kind) {
		if v, ok := args["recipient_id"].(float64); ok && int64(v) == recipient {
			n++
		}
	}
	return n
}

// The event and the job go in the transaction the scope is bound to: they commit with it and
// roll back with it.
func TestPublishInAndEnqueueInJoinTheScopesTransaction(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	scope := env.DB.Scoped(fixtures.AcmeID)
	boom := errors.New("producer failed")

	err := env.Bus.WithinScopedTx(ctx, scope, func(ts *ent.Scoped, _ events.Publisher) error {
		require.NoError(t, publishAndEnqueueIn(ctx, env.Bus, ts, "in-rolled-back@example.com", 901))
		return boom
	})
	require.ErrorIs(t, err, boom)
	assert.Zero(t, publishedTo(t, env, "in-rolled-back@example.com"), "the event rolls back with the transaction")
	assert.Zero(t, queuedSends(t, env, 901), "and so does the job")

	require.NoError(t, env.Bus.WithinScopedTx(ctx, scope, func(ts *ent.Scoped, _ events.Publisher) error {
		return publishAndEnqueueIn(ctx, env.Bus, ts, "in-committed@example.com", 902)
	}))
	assert.Equal(t, 1, publishedTo(t, env, "in-committed@example.com"), "the event commits with the transaction")
	assert.Equal(t, 1, queuedSends(t, env, 902), "and so does the job")
}

// Every client a transaction of the bus hands out reaches its transaction: the tx-bound client,
// both scopes of a hand-over and the scope that holds the Workspace lock.
func TestPublishInReachesEveryClientOfABusTransaction(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	scope := env.DB.Scoped(fixtures.AcmeID)

	require.NoError(t, env.Bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		return publishAndEnqueueIn(ctx, env.Bus, tx, "in-client@example.com", 911)
	}))
	require.NoError(t, env.Bus.WithinHandOverTx(ctx, scope, fixtures.InitechID, func(to, from *ent.Scoped, _ events.Publisher) error {
		if err := publishAndEnqueueIn(ctx, env.Bus, to, "in-to@example.com", 912); err != nil {
			return err
		}
		return publishAndEnqueueIn(ctx, env.Bus, from, "in-from@example.com", 913)
	}))
	require.NoError(t, env.Bus.WithinWorkspaceLockTx(ctx, scope, func(l *ent.WorkspaceLocked, _ events.Publisher) error {
		return publishAndEnqueueIn(ctx, env.Bus, l, "in-locked@example.com", 914)
	}))

	for email, recipient := range map[string]int64{
		"in-client@example.com": 911, "in-to@example.com": 912, "in-from@example.com": 913, "in-locked@example.com": 914,
	} {
		assert.Equal(t, 1, publishedTo(t, env, email), email)
		assert.Equal(t, 1, queuedSends(t, env, recipient), email)
	}
}

// PublishIn publishes through the transaction's own publisher: what the bus runs once an Event is
// stored runs once that transaction committed.
func TestPublishInRunsOnStoredAfterTheCommit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	var stored []string
	env.Bus.OnStored(func(_ context.Context, ev events.DomainEvent) {
		if c, ok := ev.(*events.ContactCreated); ok {
			stored = append(stored, c.Email)
		}
	})

	require.NoError(t, env.Bus.WithinScopedTx(ctx, env.DB.Scoped(fixtures.AcmeID), func(ts *ent.Scoped, _ events.Publisher) error {
		require.NoError(t, env.Bus.PublishIn(ctx, ts, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Email: "in-stored@example.com"}))
		assert.Empty(t, stored, "nothing runs before the commit")
		return nil
	}))
	assert.Equal(t, []string{"in-stored@example.com"}, stored)
}

// A client bound to no transaction of the bus is refused: the bus never opens one for it.
func TestPublishInRefusesAClientOutsideABusTransaction(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	other := events.New(env.SQLDB, entclient.Core.NewTx, time.Now)

	refused := func(t *testing.T, in events.InTx) {
		t.Helper()
		require.ErrorIs(t, publishAndEnqueueIn(ctx, env.Bus, in, "in-refused@example.com", 921), events.ErrNotInBusTx)
		require.ErrorIs(t, env.Bus.EnqueueIn(ctx, in, jobs.SendRecipientArgs{WorkspaceID: fixtures.AcmeID, RecipientID: 921, BroadcastID: 1}, nil), events.ErrNotInBusTx)
	}
	t.Run("the pool's client", func(t *testing.T) { refused(t, env.DB) })
	t.Run("a scope over the pool", func(t *testing.T) { refused(t, env.DB.Scoped(fixtures.AcmeID)) })
	t.Run("a transaction of another bus", func(t *testing.T) {
		require.NoError(t, other.WithinScopedTx(ctx, env.DB.Scoped(fixtures.AcmeID), func(ts *ent.Scoped, _ events.Publisher) error {
			refused(t, ts)
			return nil
		}))
	})
	assert.Zero(t, publishedTo(t, env, "in-refused@example.com"))
	assert.Zero(t, queuedSends(t, env, 921))
}
