package events_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/eventjournalentry"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/logging"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestJournalKeepsUnconsumedFactsAndInstanceFactsWithoutTheirPersonalPayload(t *testing.T) {
	env := testhelper.Setup(t)
	at := time.Date(2026, 11, 8, 1, 0, 0, 0, time.UTC)
	bus := events.New(env.SQLDB, entclient.NewTx, func() time.Time { return at })
	ctx := logging.WithRequestID(t.Context(), "request-with-private-data@example.com")
	for _, fact := range []events.DomainEvent{
		&events.InvitationCreated{WorkspaceID: fixtures.AcmeID, Email: "invitee@example.com", Token: "secret-invitation-token"},
		&events.LinkRequested{Action: events.NameUserPasswordResetRequested, Email: "account@example.com", BrowserHash: "private-browser"},
	} {
		require.NoError(t, bus.Publish(ctx, fact), "no consumer and no queue are needed to retain a publication")
	}
	rows := env.DB.EventJournalEntry.Query().Where(eventjournalentry.NameIn(events.NameInvitationCreated, events.NameUserPasswordResetRequested)).Order(eventjournalentry.ByID()).AllX(t.Context())
	require.Len(t, rows, 2)
	assert.Equal(t, int64(fixtures.AcmeID), *rows[0].WorkspaceID)
	assert.Nil(t, rows[1].WorkspaceID, "an Instance fact is not assigned to an invented Workspace")
	for _, row := range rows {
		assert.True(t, at.Equal(row.OccurredAt))
		assert.True(t, at.Equal(row.RecordedAt))
		assert.Empty(t, row.Inputs)
		encoded, err := json.Marshal(row)
		require.NoError(t, err)
		for _, secret := range []string{"@example.com", "secret-invitation-token", "private-browser", "request-with-private-data"} {
			assert.NotContains(t, string(encoded), secret)
		}
	}
}

func TestJournalSharesDuplicateAndRollbackSemanticsWithPublication(t *testing.T) {
	env := testhelper.Setup(t)
	fact := &internalObservation{WorkspaceID: fixtures.AcmeID, SourceID: "private-source@example.com"}
	boom := errors.New("mandatory failure")
	fail := true
	env.Bus.UseSubscribers(events.Subscriber{
		Wants: func(_ events.Envelope, ev events.DomainEvent) bool { _, ok := ev.(*internalObservation); return ok },
		Handle: func(context.Context, *ent.Scoped, events.Publisher, events.Envelope, events.DomainEvent) error {
			if fail {
				return boom
			}
			return nil
		},
	})
	count := func() int {
		return env.DB.EventJournalEntry.Query().Where(eventjournalentry.Name(fact.EventName())).CountX(t.Context())
	}
	require.ErrorIs(t, env.Bus.Publish(t.Context(), fact), boom)
	assert.Zero(t, count())
	fail = false
	require.NoError(t, env.Bus.WithinTx(t.Context(), func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(t.Context(), fact)
	}))
	require.NoError(t, env.Bus.Publish(t.Context(), fact))
	assert.Equal(t, 1, count(), "the failed attempt consumed nothing; the redelivery adds nothing")
	row := env.DB.EventJournalEntry.Query().Where(eventjournalentry.Name(fact.EventName())).OnlyX(t.Context())
	encoded, err := json.Marshal(row)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), fact.SourceID)

	other := &internalObservation{WorkspaceID: fixtures.AcmeID, SourceID: "another-source"}
	require.ErrorIs(t, env.Bus.WithinTx(t.Context(), func(_ *ent.Client, pub events.Publisher) error {
		if err := pub.Publish(t.Context(), other); err != nil {
			return err
		}
		return boom
	}), boom)
	assert.Equal(t, 1, count(), "the producer's own rollback removes its journal write")
}

func TestJournalFailureRollsBackTheProducerAndItsQueuedWork(t *testing.T) {
	env := testhelper.Setup(t)
	boom := errors.New("journal unavailable")
	env.Bus.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if mutation.Type() == "EventJournalEntry" {
				return nil, boom
			}
			return next.Mutate(ctx, mutation)
		})
	})
	before := env.DB.Contact.GetX(t.Context(), fixtures.ContactAliceID)
	err := env.Bus.WithinTx(t.Context(), func(tx *ent.Client, pub events.Publisher) error {
		// test writes: publication's failure must undo its producer's state change
		if err := tx.Contact.UpdateOneID(fixtures.ContactAliceID).SetFirstName("Rolled back").Exec(t.Context()); err != nil {
			return err
		}
		publishErr := pub.Publish(t.Context(), &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID})
		assert.ErrorIs(t, publishErr, boom)
		return nil
	})
	require.ErrorIs(t, err, boom, "even an ignored publication failure prevents commit")
	assert.Equal(t, before.FirstName, env.DB.Contact.GetX(t.Context(), fixtures.ContactAliceID).FirstName)
	assert.Empty(t, env.QueuedEvents(t, events.NameContactCreated))
	assert.Empty(t, env.DB.EventJournalEntry.Query().Where(eventjournalentry.WorkspaceID(fixtures.AcmeID), eventjournalentry.Name(events.NameContactCreated)).IDsX(t.Context()))
}

func TestJournalErasureIsBatchedAndConfinedToItsWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	for range 3 {
		require.NoError(t, env.Bus.Publish(t.Context(), &events.SecondFactorRequired{WorkspaceID: fixtures.AcmeID}))
	}
	require.NoError(t, env.Bus.Publish(t.Context(), &events.SecondFactorRequired{WorkspaceID: fixtures.InitechID}))
	require.NoError(t, env.Bus.Publish(t.Context(), &events.PasskeyChange{Action: events.NameUserPasskeyAdded}))
	for _, want := range []int{2, 1, 0} {
		n, err := env.Bus.EraseWorkspaceJournalBatch(t.Context(), env.DB.Scoped(fixtures.AcmeID), 2)
		require.NoError(t, err)
		assert.Equal(t, want, n)
	}
	assert.Equal(t, 1, env.DB.EventJournalEntry.Query().Where(eventjournalentry.WorkspaceID(fixtures.InitechID)).CountX(t.Context()))
	assert.Equal(t, 1, env.DB.EventJournalEntry.Query().Where(eventjournalentry.WorkspaceIDIsNil()).CountX(t.Context()))
}

func TestJournalPublicationAndFinalWorkspacePurgeCannotLeaveLateHistory(t *testing.T) {
	database := testhelper.RealDB(t)
	database.SetMaxOpenConns(4)
	client := entclient.New(database)
	bus := testhelper.BusOn(t, database)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	now := time.Now()
	require.NoError(t, bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		return tx.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(now.Add(-workspaces.DeletionWindow - time.Hour)).Exec(ctx)
	}))
	fact := &internalObservation{WorkspaceID: fixtures.AcmeID, SourceID: "accepted-before-tombstone"}
	unblock, published := pauseJournalPublication(ctx, t, bus, fact)
	results := testhelper.QueuedBehind(t, database, unblock, testhelper.Writer{
		Name: "final purge", Run: func() error {
			return bus.WithinWorkspacePurgeTx(ctx, func(tx *ent.Client, purger events.WorkspacePurger) error {
				if err := purger.PurgeWorkspaceJobs(ctx, fixtures.AcmeID); err != nil {
					return err
				}
				return workspaces.TombstoneIn(ctx, tx, fixtures.AcmeID, now)
			})
		},
	})
	require.NoError(t, results["final purge"])
	require.NoError(t, testhelper.Receive(t, published, "publisher committed"))
	assert.Zero(t, client.EventJournalEntry.Query().Where(eventjournalentry.WorkspaceID(fixtures.AcmeID)).CountX(ctx))
	assert.Empty(t, testhelper.QueuedEnvelopesOn(t, database, fact.EventName()))
	fact.SourceID = "arrived-after-tombstone"
	require.ErrorIs(t, bus.Publish(ctx, fact), events.ErrWorkspaceErased)
}

func TestJournalPublicationDoesNotBlockAnOwnersFeedUpdate(t *testing.T) {
	database := testhelper.RealDB(t)
	database.SetMaxOpenConns(4)
	bus := testhelper.BusOn(t, database)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	unblock, published := pauseJournalPublication(ctx, t, bus, &internalObservation{WorkspaceID: fixtures.AcmeID, SourceID: "held-during-owner-update"})
	updated := make(chan error, 1)
	go func() {
		updated <- bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
			return tx.User.UpdateOneID(fixtures.OwnerJohnID).SetEmail("owner-changed@example.com").Exec(ctx)
		})
	}()
	require.NoError(t, testhelper.Receive(t, updated, "owner email and feed position change while publication is open"))
	unblock()
	require.NoError(t, testhelper.Receive(t, published, "publisher committed"))
}

func pauseJournalPublication(ctx context.Context, t *testing.T, bus *events.Bus, fact events.DomainEvent) (resume func(), done <-chan error) {
	t.Helper()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	bus.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if mutation.Type() == "EventJournalEntry" && mutation.Op().Is(ent.OpCreate) {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return next.Mutate(ctx, mutation)
		})
	})
	published := make(chan error, 1)
	go func() { published <- bus.Publish(ctx, fact) }()
	testhelper.Receive(t, entered, "publisher reached its journal write")
	return unblock, published
}
