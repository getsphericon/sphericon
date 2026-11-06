package outboundmessages_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/capturedmessage"
	"github.com/getsphericon/sphericon/internal/domain/sending/capturedmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/tracking"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/inbox"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// pinewoodInboxModule sends Pinewood's marketing through its default, its Test inbox: the real
// provider keeping Captured messages, with the 200 a day the Integration row reports, on a clock the
// test moves. opts add to the module's options.
func pinewoodInboxModule(t *testing.T, env *testhelper.TestEnv, c *clock, opts ...outboundmessages.Option) *outboundmessages.Module {
	t.Helper()
	s := env.DB.Scoped(fixtures.PinewoodID)
	built, err := inbox.Descriptor().Build(nil, messaging.Deps{
		Capturer: capturedmessages.New(inbox.PerDay).Capturer(s, fixtures.IntegrationPinewoodInboxID),
	})
	require.NoError(t, err)
	sender, ok := built.(messaging.EmailSender)
	require.True(t, ok)
	return outboundmessages.New(env.Bus, env.Holds, senders{sender: sender}, tracking.New("test-secret", "http://local"),
		append([]outboundmessages.Option{outboundmessages.WithClock(c.now)}, opts...)...)
}

// sendToReader sends a marketing message to Pinewood's Contact.
func sendToReader(t *testing.T, env *testhelper.TestEnv, m *outboundmessages.Module, key string) outboundmessages.Result {
	t.Helper()
	res, err := m.Send(t.Context(), env.DB.Scoped(fixtures.PinewoodID), marketing(t, env, key, fixtures.ContactPinewoodReaderID))
	require.NoError(t, err)
	return res
}

// The Test inbox accepts 200 messages in a rolling 24 hours: the 201st of a Broadcast or an
// Automation is Deferred like any rate-limited send, and is sent once capacity returns.
func TestTestInboxDefersTheMessageAfterItsDailyCeilingAndSendsItLater(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := startClock()
	m := pinewoodInboxModule(t, env, c)
	s := env.DB.Scoped(fixtures.PinewoodID)
	sendTo := func(key string) outboundmessages.Result { return sendToReader(t, env, m, key) }

	for i := range inbox.PerDay {
		require.Equalf(t, outboundmessages.Sent, sendTo(fmt.Sprintf("bc:%d", i)).Outcome, "message %d", i+1)
	}

	res := sendTo("bc:201")
	require.Equal(t, outboundmessages.Deferral, res.Outcome, "the 201st is Deferred, not lost or failed")
	assert.Equal(t, 24*time.Hour/inbox.PerDay, res.Wait, "one token of 200 a day returns every 7.2 minutes")
	assert.Zero(t, res.MessageID, "a Deferral leaves no Outbound message")

	c.elapse(res.Wait)
	assert.Equal(t, outboundmessages.Sent, sendTo("bc:201").Outcome, "sent later with the same key")
	kept := s.CapturedMessage().Query().Where(capturedmessage.IntegrationID(fixtures.IntegrationPinewoodInboxID)).CountX(ctx)
	assert.GreaterOrEqual(t, kept, inbox.PerDay, "every accepted message was captured")
}

// A lower limit the user sets on the Test inbox applies as for any Integration.
func TestTestInboxHonoursALowerUserLimit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := startClock()
	m := pinewoodInboxModule(t, env, c)
	env.DB.Integration.UpdateOneID(fixtures.IntegrationPinewoodInboxID).SetMaxPerDay(2).ExecX(ctx)
	sendTo := func(key string) outboundmessages.Result { return sendToReader(t, env, m, key) }

	require.Equal(t, outboundmessages.Sent, sendTo("lo:1").Outcome)
	require.Equal(t, outboundmessages.Sent, sendTo("lo:2").Outcome)

	assert.Equal(t, outboundmessages.Deferral, sendTo("lo:3").Outcome)
}
