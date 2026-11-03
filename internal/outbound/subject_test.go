package outbound_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Outbound message keeps the subject the Contact received, merge tags filled (#899), so a
// feed can name the email an Event is about after the template has changed or gone.
func TestASentEmailKeepsItsRenderedSubject(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	res, err := newModule(env).Send(ctx, s, outbound.Transactional(outbound.TransactionalMessage{
		Destination: "ada@example.com",
		Variables:   map[string]any{"nick": "countess"},
		Content:     outbound.EmailContent{Subject: "Hi {{ nick }}", Body: htmlBody},
	}))
	require.NoError(t, err)
	require.Equal(t, outbound.Sent, res.Outcome)

	msg := s.OutboundMessage().Query().Where(outboundmessage.ID(res.MessageID)).OnlyX(ctx)
	require.NotNil(t, msg.Subject)
	assert.Equal(t, "Hi countess", *msg.Subject)
}
