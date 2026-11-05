package events_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Publishing an EmailDelivery with a long natural id must succeed: the
// id becomes the envelope DedupKey and the Event's source id, while the envelope keeps a short
// ULID of its own.
func TestPublishDeliveryFailureWithLongDedupID(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	err := env.Bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(ctx, &events.EmailDelivery{
			Action:      events.NameEmailBounced,
			WorkspaceID: fixtures.AcmeID,
			Email:       "recipient@example.com",
			BounceKind:  events.BounceKindHard,
			Provider:    "ses",
			// SNS messageId (36-char UUID) + "/" + email — well over 36 chars.
			DedupID: "9c8e1f2a-3b4d-4e5f-8a9b-0c1d2e3f4a5b/recipient@example.com",
		})
	})
	require.NoError(t, err)
}
