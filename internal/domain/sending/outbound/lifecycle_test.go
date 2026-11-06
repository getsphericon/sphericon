package outbound_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// sent, skipped and failed are final (ADR 0046): giving up on such a message changes nothing,
// though its claim is long past the lease that would otherwise let it be failed.
func TestAFinalMessageIsNeverMarkedFailed(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	for _, status := range []outboundmessage.Status{
		outboundmessage.StatusSent, outboundmessage.StatusSkipped, outboundmessage.StatusFailed,
	} {
		before := s.OutboundMessage().Query().
			Where(outboundmessage.StatusEQ(status)).
			Order(ent.Asc(outboundmessage.FieldID)).
			FirstX(ctx)

		require.NoError(t, newModule(env).MarkFailed(ctx, s, before.IdempotencyKey, errors.New("gave up")))

		after := s.OutboundMessage().Query().Where(outboundmessage.ID(before.ID)).OnlyX(ctx)
		assert.Equal(t, status, after.Status)
		assert.Equal(t, before.Reason, after.Reason)
	}
}
