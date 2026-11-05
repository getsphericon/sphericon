package jobs_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// suppressEvent works the suppression consumer's job for env, as River hands it to the worker.
func suppressEvent(ctx context.Context, client *ent.Client, env events.Envelope) error {
	return jobs.NewSuppressEventWorker(client).Work(ctx, &river.Job[jobs.SuppressEventArgs]{
		Args: jobs.SuppressEventArgs{EventJob: events.EventJob{Envelope: env}},
	})
}

// dataFor marshals a typed event into the envelope Data, as the publisher does.
func dataFor(t *testing.T, ev events.DomainEvent) []byte {
	t.Helper()
	b, err := json.Marshal(ev)
	require.NoError(t, err)
	return b
}

func envelopeOf(name, data string) events.Envelope {
	return events.Envelope{ID: "evt", Name: name, Version: 1, WorkspaceID: 1, Data: json.RawMessage(data)}
}

// The suppression consumer refuses an envelope it cannot decode.
func TestSuppressRejectsUndecodableEnvelopes(t *testing.T) {
	ctx := t.Context()
	require.ErrorContains(t, suppressEvent(ctx, nil, envelopeOf("no.such.event", "{}")), "unknown event")
	require.ErrorContains(t, suppressEvent(ctx, nil, envelopeOf(events.NameContactCreated, `"scalar"`)), "decode")
}

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
	require.NoError(t, suppressEvent(ctx, env.DB, envlp))

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
	require.NoError(t, suppressEvent(ctx, env.DB, envlp))

	exists, err := env.DB.Suppression.Query().
		Where(suppression.WorkspaceID(fixtures.AcmeID), suppression.Destination("open@example.com")).
		Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists)
}

// A hard bounce suppresses the address with reason "bounce".
func TestSuppressOnHardBounce(t *testing.T) {
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
			BounceKind:  events.BounceKindHard,
			Provider:    "ses",
		}),
	}
	require.NoError(t, suppressEvent(ctx, env.DB, envlp))

	s, err := env.DB.Suppression.Query().
		Where(suppression.WorkspaceID(fixtures.AcmeID),
			suppression.ChannelEQ(channels.Email),
			suppression.DestinationEQ("hardbounce@example.com")).
		Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, suppression.ReasonBounce, s.Reason)
}

// A soft bounce does NOT suppress — it is temporary.
func TestSuppressIgnoresSoftBounce(t *testing.T) {
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
			BounceKind:  events.BounceKindSoft,
			Provider:    "ses",
		}),
	}
	require.NoError(t, suppressEvent(ctx, env.DB, envlp))

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
	require.NoError(t, suppressEvent(ctx, env.DB, envlp))

	s, err := env.DB.Suppression.Query().
		Where(suppression.WorkspaceID(fixtures.AcmeID),
			suppression.ChannelEQ(channels.Email),
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
	require.NoError(t, suppressEvent(ctx, env.DB, envlp))
	require.NoError(t, suppressEvent(ctx, env.DB, envlp)) // redelivery

	n, err := env.DB.Suppression.Query().
		Where(suppression.WorkspaceID(fixtures.AcmeID),
			suppression.ChannelEQ(channels.Email),
			suppression.DestinationEQ("dupe@example.com")).
		Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}
