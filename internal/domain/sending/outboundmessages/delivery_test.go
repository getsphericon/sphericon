package outboundmessages_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A final report lands only on a message the provider accepted (ADR 0079): one still pending has
// no final state to take, whichever report names it.
func TestAFinalReportStampsNothingOnAMessageThatWasNotSent(t *testing.T) {
	for _, result := range []outboundmessages.DeliveryResult{outboundmessages.ResultDelivered, outboundmessages.ResultUndelivered} {
		t.Run(string(result), func(t *testing.T) {
			env := testhelper.Setup(t)

			moved, err := outboundmessages.StampFinal(t.Context(), env.DB.Scoped(fixtures.AcmeID), fixtures.OutboundMessageStaleClaimID, result, "30003", time.Now())

			require.NoError(t, err)
			assert.False(t, moved)
			got := env.DB.OutboundMessage.GetX(t.Context(), fixtures.OutboundMessageStaleClaimID)
			assert.Nil(t, got.DeliveredAt)
			assert.Nil(t, got.UndeliveredAt)
			assert.Nil(t, got.UndeliveredCode)
		})
	}
}

// A message's final report is delivered or undelivered, never both: the table refuses the second,
// whoever writes it.
func TestTheDatabaseRefusesAMessageBothDeliveredAndUndelivered(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	moved, err := outboundmessages.StampFinal(t.Context(), s, fixtures.OutboundTexterSmsID, outboundmessages.ResultDelivered, "", time.Now())
	require.NoError(t, err)
	require.True(t, moved)

	err = env.DB.OutboundMessage.UpdateOneID(fixtures.OutboundTexterSmsID).SetUndeliveredAt(time.Now()).Exec(t.Context())

	require.Error(t, err)
	assert.True(t, ent.IsConstraintError(err), "got %v", err)
}
