package events_test

import (
	"context"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An unsubscribe is NOT a suppression (ADR 0001): it is a separate, per-source
// opt-out, so an unsubscribe event must not create a suppression row.
func TestSuppressIgnoresUnsubscribe(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	envlp := events.Envelope{
		ID:          "01J0SUPPRESS00000000000000",
		Name:        events.NameEmailUnsubscribed,
		Version:     1,
		WorkspaceID: fixtures.AcmeID,
		OccurredAt:  time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC),
		Data: dataFor(t, &events.EmailEngagement{
			Action:      events.NameEmailUnsubscribed,
			WorkspaceID: fixtures.AcmeID,
			ContactID:   7,
			Email:       "Unsub@Example.com",
		}),
	}
	require.NoError(t, events.Suppress(ctx, env.DB, envlp))

	// No suppression was created for this destination (unsubscribe is not a
	// suppression reason). Scoped to the destination so unrelated fixtures don't matter.
	exists, err := env.DB.Suppression.Query().
		Where(suppression.WorkspaceID(fixtures.AcmeID), suppression.Destination("unsub@example.com")).
		Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists, "unsubscribe must not suppress")
}

// Opens/clicks (and any non-suppressing action) do not create a suppression.
func TestSuppressIgnoresNonSuppressingActions(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	envlp := events.Envelope{
		ID:          "01J0SUPPRESS00000000000001",
		Name:        events.NameEmailOpened,
		Version:     1,
		WorkspaceID: fixtures.AcmeID,
		OccurredAt:  time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC),
		Data: dataFor(t, &events.EmailEngagement{
			Action:      events.NameEmailOpened,
			WorkspaceID: fixtures.AcmeID,
			ContactID:   7,
			Email:       "open@example.com",
		}),
	}
	require.NoError(t, events.Suppress(ctx, env.DB, envlp))

	exists, err := env.DB.Suppression.Query().
		Where(suppression.WorkspaceID(fixtures.AcmeID), suppression.Destination("open@example.com")).
		Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists)
}

// Publishing an EmailDelivery with a long natural id must succeed: the
// id becomes the envelope DedupKey, while the (short ULID) message id fills the
// outbox uuid column. Regression test for overflowing that column with the id.
func TestPublishDeliveryFailureWithLongDedupID(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	err := env.Bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(ctx, &events.EmailDelivery{
			Action:      events.NameEmailBounced,
			WorkspaceID: fixtures.AcmeID,
			Email:       "recipient@example.com",
			BounceKind:  events.BounceKindPermanent,
			Provider:    "ses",
			// SNS messageId (36-char UUID) + "/" + email — well over 36 chars.
			DedupID: "9c8e1f2a-3b4d-4e5f-8a9b-0c1d2e3f4a5b/recipient@example.com",
		})
	})
	require.NoError(t, err)
}

// The DedupKey makes persist idempotent across at-least-once redelivery.
func TestPersistDedupesOnDedupKey(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	envlp := events.Envelope{
		ID:          "01J0DELIVERYFAILURE0000001",
		Name:        events.NameEmailBounced,
		Version:     1,
		WorkspaceID: fixtures.AcmeID,
		OccurredAt:  time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC),
		DedupKey:    "sns-msg-1/bounce@example.com",
		Data: dataFor(t, &events.EmailDelivery{
			Action: events.NameEmailBounced, WorkspaceID: fixtures.AcmeID,
			Email: "bounce@example.com", BounceKind: events.BounceKindPermanent,
		}),
	}
	// Same DedupKey, different envelope ID (a redelivery) → one persisted row.
	require.NoError(t, events.Persist(ctx, env.DB, envlp))
	envlp.ID = "01J0DELIVERYFAILURE0000002"
	require.NoError(t, events.Persist(ctx, env.DB, envlp))

	n, err := env.DB.Event.Query().Where(event.SourceID(envlp.DedupKey)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "same DedupKey must persist at most once")
}

// A permanent bounce suppresses the address with reason "bounce".
func TestSuppressOnPermanentBounce(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	envlp := events.Envelope{
		ID:          "01J0SUPPRESS00000000000010",
		Name:        events.NameEmailBounced,
		Version:     1,
		WorkspaceID: fixtures.AcmeID,
		OccurredAt:  time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC),
		Data: dataFor(t, &events.EmailDelivery{
			Action:      events.NameEmailBounced,
			WorkspaceID: fixtures.AcmeID,
			Email:       "hardbounce@example.com",
			BounceKind:  events.BounceKindPermanent,
			Provider:    "ses",
		}),
	}
	require.NoError(t, events.Suppress(ctx, env.DB, envlp))

	s, err := env.DB.Suppression.Query().
		Where(suppression.WorkspaceID(fixtures.AcmeID),
			suppression.ChannelEQ(suppression.ChannelEmail),
			suppression.DestinationEQ("hardbounce@example.com")).
		Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, suppression.ReasonBounce, s.Reason)
}

// A transient (soft) bounce does NOT suppress — it is temporary.
func TestSuppressIgnoresTransientBounce(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	envlp := events.Envelope{
		ID:          "01J0SUPPRESS00000000000011",
		Name:        events.NameEmailBounced,
		Version:     1,
		WorkspaceID: fixtures.AcmeID,
		OccurredAt:  time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC),
		Data: dataFor(t, &events.EmailDelivery{
			Action:      events.NameEmailBounced,
			WorkspaceID: fixtures.AcmeID,
			Email:       "softbounce@example.com",
			BounceKind:  events.BounceKindTransient,
			Provider:    "ses",
		}),
	}
	require.NoError(t, events.Suppress(ctx, env.DB, envlp))

	exists, err := env.DB.Suppression.Query().
		Where(suppression.WorkspaceID(fixtures.AcmeID), suppression.Destination("softbounce@example.com")).
		Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists)
}

// A spam complaint suppresses the address with reason "complaint".
func TestSuppressOnComplaint(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	envlp := events.Envelope{
		ID:          "01J0SUPPRESS00000000000012",
		Name:        events.NameEmailComplained,
		Version:     1,
		WorkspaceID: fixtures.AcmeID,
		OccurredAt:  time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC),
		Data: dataFor(t, &events.EmailDelivery{
			Action:      events.NameEmailComplained,
			WorkspaceID: fixtures.AcmeID,
			Email:       "complaint@example.com",
			Provider:    "ses",
		}),
	}
	require.NoError(t, events.Suppress(ctx, env.DB, envlp))

	s, err := env.DB.Suppression.Query().
		Where(suppression.WorkspaceID(fixtures.AcmeID),
			suppression.ChannelEQ(suppression.ChannelEmail),
			suppression.DestinationEQ("complaint@example.com")).
		Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, suppression.ReasonComplaint, s.Reason)
}

// Redelivery or a repeat suppressing event keeps a single entry (idempotent upsert).
func TestSuppressIsIdempotent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	envlp := events.Envelope{
		ID:          "01J0SUPPRESS00000000000002",
		Name:        events.NameEmailComplained,
		Version:     1,
		WorkspaceID: fixtures.AcmeID,
		OccurredAt:  time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC),
		Data: dataFor(t, &events.EmailDelivery{
			Action:      events.NameEmailComplained,
			WorkspaceID: fixtures.AcmeID,
			Email:       "dupe@example.com",
		}),
	}
	require.NoError(t, events.Suppress(ctx, env.DB, envlp))
	require.NoError(t, events.Suppress(ctx, env.DB, envlp)) // redelivery

	n, err := env.DB.Suppression.Query().
		Where(suppression.WorkspaceID(fixtures.AcmeID),
			suppression.ChannelEQ(suppression.ChannelEmail),
			suppression.DestinationEQ("dupe@example.com")).
		Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}
