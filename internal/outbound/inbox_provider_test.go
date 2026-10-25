package outbound_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// keepingSender is a provider that delivers nowhere outside and says whether it
// confirmed delivery when it accepted a message: what a built-in test provider
// reports about itself, observed at the Outbound send seam.
type keepingSender struct {
	confirms bool
	sent     []messaging.EmailMessage
}

func (k *keepingSender) Send(_ context.Context, msg messaging.EmailMessage) (messaging.Receipt, error) {
	k.sent = append(k.sent, msg)
	return messaging.Receipt{MessageID: "kept-1", DeliveryConfirmed: k.confirms}, nil
}

func (*keepingSender) DeliversOutside() bool { return false }

func moduleThrough(env *testhelper.TestEnv, sender messaging.EmailSender) *outbound.Module {
	return outbound.New(env.Bus, env.Holds, senders{sender: sender},
		tracking.New("test-secret", "http://local"))
}

func TestSenderThatDeliversNowhereIsSentThroughWithoutAVerifiedSendingDomain(t *testing.T) {
	env := testhelper.Setup(t)
	kept := &keepingSender{}

	req := transactional("tx:kept", "a@example.com")
	req.FromEmail = "noreply@news.acme.com" // fixture domain id 2: unverified
	res, err := moduleThrough(env, kept).Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), req)
	require.NoError(t, err)

	assert.Equal(t, outbound.Sent, res.Outcome)
	require.Len(t, kept.sent, 1)
	assert.Equal(t, "noreply@news.acme.com", kept.sent[0].From, "the From is carried through as given")
}

func TestSenderThatConfirmsOnAcceptanceIsStampedDeliveredOnce(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := moduleThrough(env, &keepingSender{confirms: true})

	res, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:confirmed", "a@example.com"))
	require.NoError(t, err)
	again, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:confirmed", "a@example.com"))
	require.NoError(t, err)
	assert.True(t, again.Replayed)

	row := env.DB.OutboundMessage.GetX(ctx, res.MessageID)
	assert.NotNil(t, row.DeliveredAt)
	assert.Nil(t, row.BouncedAt)
	assert.Nil(t, row.ComplainedAt)
	assert.Equal(t, 1, env.OutboxCount(t, "email.delivered", map[string]any{"outboundMessageId": res.MessageID}))
}

func TestSenderThatDoesNotConfirmIsNotStampedDelivered(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	res, err := moduleThrough(env, &keepingSender{}).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:plain", "a@example.com"))
	require.NoError(t, err)

	assert.Nil(t, env.DB.OutboundMessage.GetX(ctx, res.MessageID).DeliveredAt)
	assert.Zero(t, env.OutboxCount(t, "email.delivered", map[string]any{"outboundMessageId": res.MessageID}))
}
