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
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func publishedCount(t *testing.T, env *testhelper.TestEnv) int {
	t.Helper()
	return len(env.QueuedEnvelopes(t))
}

// dataFor marshals a typed event into the envelope Data, as the publisher does.
func dataFor(t *testing.T, ev events.DomainEvent) []byte {
	t.Helper()
	b, err := json.Marshal(ev)
	require.NoError(t, err)
	return b
}

// The load-bearing requirement: the state row and the event commit together.
func TestBusWithinTxCommitsStateAndEvent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	before := publishedCount(t, env)

	var created *ent.Contact
	err := env.Bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		// test writes: transaction commit and rollback through the bus is the subject
		c, err := tx.Contact.Create().
			SetWorkspaceID(fixtures.AcmeID).
			SetEmail("bus-commit@example.com").
			Save(ctx)
		if err != nil {
			return err
		}
		created = c
		return pub.Publish(ctx, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: c.ID, Email: *c.Email})
	})
	require.NoError(t, err)

	// State row committed (visible through the shared txdb connection)...
	got, err := env.DB.Contact.Query().Where(contact.Email("bus-commit@example.com")).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	// ...and exactly one event was published in the same transaction.
	assert.Equal(t, before+1, publishedCount(t, env))
}

// If the unit of work fails, neither the state nor the event is published.
func TestBusWithinTxRollsBackBoth(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	before := publishedCount(t, env)

	boom := errors.New("boom")
	err := env.Bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		// test writes: transaction commit and rollback through the bus is the subject
		if _, err := tx.Contact.Create().
			SetWorkspaceID(fixtures.AcmeID).
			SetEmail("bus-rollback@example.com").
			Save(ctx); err != nil {
			return err
		}
		if err := pub.Publish(ctx, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, Email: "bus-rollback@example.com"}); err != nil {
			return err
		}
		return boom
	})
	require.ErrorIs(t, err, boom)

	exists, err := env.DB.Contact.Query().Where(contact.Email("bus-rollback@example.com")).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists, "contact must not be committed when the unit of work fails")
	assert.Equal(t, before, publishedCount(t, env), "no event published on rollback")
	stored, err := env.DB.Event.Query().Where(event.Email("bus-rollback@example.com")).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, stored, "no Event stored on rollback")
}

// publishAlone publishes ev in a bus transaction of its own.
func publishAlone(t *testing.T, env *testhelper.TestEnv, ev events.DomainEvent) {
	t.Helper()
	require.NoError(t, env.Bus.WithinTx(t.Context(), func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(t.Context(), ev)
	}))
}

// Publishing an Event writes its row in the producer's transaction: every column, the resolved
// contact_id, visitor_id, phone, a non-now OccurredAt and the opaque Properties included. A
// CollectedEvent exercises them all.
func TestPublishStoresTheEvent(t *testing.T) {
	env := testhelper.Setup(t)
	occurred := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)

	publishAlone(t, env, &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID,
		ContactID:   fixtures.ContactAliceID,
		VisitorID:   "visitor-abc",
		SubjectID:   "visitor:abc",
		Action:      "page_view",
		Email:       "persist@example.com",
		Phone:       "+15550100",
		Properties:  map[string]any{"path": "/pricing"},
		OccurredAt:  occurred,
		DedupID:     "store-projection",
	})

	row, err := env.DB.Event.Query().Where(event.SourceID("store-projection")).Only(t.Context())
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

// An Event outlives its Contact (GLOSSARY: Event): an event about a Contact that is gone, or that
// was never this Workspace's, is stored unlinked instead of failing on the contact_id foreign key.
func TestPublishStoresAnEventUnlinkedWhenItsContactIsGone(t *testing.T) {
	env := testhelper.Setup(t)

	for id, contactID := range map[string]int64{
		// A Contact that is gone: its id no longer exists.
		"store-gone": 999999,
		// Globex's contact is not an Acme contact.
		"store-foreign": fixtures.ContactGlobexID,
	} {
		publishAlone(t, env, &events.CollectedEvent{WorkspaceID: fixtures.AcmeID, ContactID: contactID, Action: "page_view", DedupID: id})
		row, err := env.DB.Event.Query().Where(event.SourceID(id)).Only(t.Context())
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

// An event whose source id is already stored (a provider's redelivery) is a repeat: it is not an
// error, and it is neither stored nor published again.
func TestARepeatedEventIsStoredAndPublishedOnce(t *testing.T) {
	env := testhelper.Setup(t)
	before := publishedCount(t, env)
	bounce := &events.EmailDelivery{
		Action: events.NameEmailBounced, WorkspaceID: fixtures.AcmeID, Email: "repeat@example.com",
		BounceKind: events.BounceKindHard, DedupID: "sns-1/repeat@example.com",
	}

	publishAlone(t, env, bounce)
	publishAlone(t, env, bounce)

	n, err := env.DB.Event.Query().Where(event.SourceID("sns-1/repeat@example.com")).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, n, "one row")
	assert.Equal(t, before+1, publishedCount(t, env), "published once")
}

// An event of the Instance (a password reset asked for) belongs to no Workspace: its envelope names
// none, it has no Event row, and no Webhook endpoint receives it, but a consumer that wants it gets
// its job.
func TestAnEventOfTheInstanceNamesNoWorkspaceAndHasNoEventRow(t *testing.T) {
	env := testhelper.Setup(t)
	stored := len(env.StoredEvents(t))

	require.NoError(t, env.Bus.Publish(t.Context(), &events.LinkRequested{
		Action: events.NameUserPasswordResetRequested, UserID: fixtures.OwnerJohnID, Email: fixtures.OwnerJohnEmail, Deliver: true,
	}))

	published := env.QueuedEnvelopes(t, events.NameUserPasswordResetRequested)
	require.Len(t, published, 1)
	assert.Zero(t, published[0].WorkspaceID)
	assert.Len(t, env.StoredEvents(t), stored, "no Event row")
	assert.Empty(t, env.EventJobs(t, jobs.WebhookEventArgs{}.Kind()), "no webhook fan-out")
	assert.Len(t, env.EventJobs(t, jobs.NoticeEventArgs{}.Kind()), 1, "the Notices' consumer wants it")
}
