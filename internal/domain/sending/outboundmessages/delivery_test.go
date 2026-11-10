package outboundmessages_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A final report lands only on a text the provider accepted (ADR 0079): one still pending has no
// final state to take, whichever report names it.
func TestAFinalReportStampsNothingOnATextThatWasNotSent(t *testing.T) {
	for _, result := range []outboundmessages.DeliveryResult{outboundmessages.ResultDelivered, outboundmessages.ResultUndelivered} {
		t.Run(string(result), func(t *testing.T) {
			env := testhelper.Setup(t)
			// test writes: no fixture text is pending; the sent one is put back before the provider's answer.
			env.DB.OutboundMessage.UpdateOneID(fixtures.OutboundTexterSmsID).SetStatus(outboundmessage.StatusPending).ExecX(t.Context())

			err := outboundmessages.StampFinal(t.Context(), env.DB.Scoped(fixtures.AcmeID), fixtures.OutboundTexterSmsID, result, "30003", time.Now())

			require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
			got := env.DB.OutboundMessage.GetX(t.Context(), fixtures.OutboundTexterSmsID)
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
	err := outboundmessages.StampFinal(t.Context(), s, fixtures.OutboundTexterSmsID, outboundmessages.ResultDelivered, "", time.Now())
	require.NoError(t, err)

	err = env.DB.OutboundMessage.UpdateOneID(fixtures.OutboundTexterSmsID).SetUndeliveredAt(time.Now()).Exec(t.Context())

	require.Error(t, err)
	assert.True(t, ent.IsConstraintError(err), "got %v", err)
}
