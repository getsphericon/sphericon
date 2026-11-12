package events_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The send path stamps EmailEngagement with a deterministic DedupID; the publisher
// must surface it as the envelope DedupKey (via the identifiable interface), which
// is what makes persist idempotent under a job retry. Open/click leave DedupID
// empty and must keep falling back to the ULID — unchanged behavior.
func TestEmailEngagementDedupIDBecomesDedupKey(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	require.NoError(t, env.Bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		if err := pub.Publish(ctx, &events.EmailEngagement{
			Action: systemevent.EmailSent, WorkspaceID: fixtures.AcmeID,
			Email: "sent@example.com", BroadcastID: fixtures.BroadcastDraftID, DedupID: "email.sent:42",
		}); err != nil {
			return err
		}
		return pub.Publish(ctx, &events.EmailEngagement{
			Action: systemevent.EmailOpened, WorkspaceID: fixtures.AcmeID,
			Email: "sent@example.com", BroadcastID: fixtures.BroadcastDraftID,
		})
	}))

	dedupOf := func(name string) string {
		t.Helper()
		msgs := env.QueuedEnvelopes(t, name)
		require.Len(t, msgs, 1)
		return msgs[0].DedupKey
	}

	assert.Equal(t, "email.sent:42", dedupOf(systemevent.EmailSent),
		"email.sent carries its DedupID as the envelope DedupKey so a retry persists once")
	assert.Empty(t, dedupOf(systemevent.EmailOpened),
		"open carries no DedupID → DedupKey stays empty (ULID fallback, unchanged)")
}
