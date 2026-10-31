package outbound_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/capturedmessage"
	"github.com/getsphericon/sphericon/internal/capturedmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/inbox"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// globexInboxModule sends Globex's marketing through its Test inbox: the real provider keeping
// Captured messages, with the 200 a day the Integration row reports, on a clock the test moves.
func globexInboxModule(t *testing.T, env *testhelper.TestEnv, c *clock) *outbound.Module {
	t.Helper()
	s := env.DB.Scoped(fixtures.GlobexID)
	built, err := inbox.Descriptor().Build(nil, messaging.Deps{
		Capturer: capturedmessages.New(inbox.PerDay).Capturer(s, fixtures.IntegrationGlobexInboxID),
	})
	require.NoError(t, err)
	sender, ok := built.(messaging.EmailSender)
	require.True(t, ok)
	env.DB.Integration.UpdateOneID(fixtures.IntegrationGlobexInboxID).SetIsDefault(true).ExecX(context.Background())
	env.DB.Integration.UpdateOneID(fixtures.IntegrationGlobexID).SetIsDefault(false).ExecX(context.Background())
	return outbound.New(env.Bus, env.Holds, senders{sender: sender}, tracking.New("test-secret", "http://local"), outbound.WithClock(c.now))
}

// sendToReader sends a marketing message to Globex's mailable Contact.
func sendToReader(t *testing.T, env *testhelper.TestEnv, m *outbound.Module, key string) outbound.Result {
	t.Helper()
	ctx := context.Background()
	reader := env.DB.Contact.GetX(ctx, fixtures.ContactGlobexReaderID)
	req := marketing(t, env, key, fixtures.ContactAliceID)
	req.Contact, req.Destination = reader, *reader.Email
	res, err := m.Send(ctx, env.DB.Scoped(fixtures.GlobexID), req)
	require.NoError(t, err)
	return res
}

// The Test inbox accepts 200 messages in a rolling 24 hours: the 201st of a Broadcast or an
// Automation is Deferred like any rate-limited send, and is sent once capacity returns.
func TestTestInboxDefersTheMessageAfterItsDailyCeilingAndSendsItLater(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := startClock()
	m := globexInboxModule(t, env, c)
	s := env.DB.Scoped(fixtures.GlobexID)
	sendTo := func(key string) outbound.Result { return sendToReader(t, env, m, key) }

	for i := range inbox.PerDay {
		require.Equalf(t, outbound.Sent, sendTo(fmt.Sprintf("bc:%d", i)).Outcome, "message %d", i+1)
	}

	res := sendTo("bc:201")
	require.Equal(t, outbound.Deferral, res.Outcome, "the 201st is Deferred, not lost or failed")
	assert.Equal(t, 24*time.Hour/inbox.PerDay, res.Wait, "one token of 200 a day returns every 7.2 minutes")
	assert.Zero(t, res.MessageID, "a Deferral leaves no Outbound message")

	c.elapse(res.Wait)
	assert.Equal(t, outbound.Sent, sendTo("bc:201").Outcome, "sent later with the same key")
	kept := s.CapturedMessage().Query().Where(capturedmessage.IntegrationID(fixtures.IntegrationGlobexInboxID)).CountX(ctx)
	assert.GreaterOrEqual(t, kept, inbox.PerDay, "every accepted message was captured")
}

// A lower limit the user sets on the Test inbox applies as for any Integration.
func TestTestInboxHonoursALowerUserLimit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := startClock()
	m := globexInboxModule(t, env, c)
	env.DB.Integration.UpdateOneID(fixtures.IntegrationGlobexInboxID).SetMaxPerDay(2).ExecX(ctx)
	sendTo := func(key string) outbound.Result { return sendToReader(t, env, m, key) }

	require.Equal(t, outbound.Sent, sendTo("lo:1").Outcome)
	require.Equal(t, outbound.Sent, sendTo("lo:2").Outcome)

	assert.Equal(t, outbound.Deferral, sendTo("lo:3").Outcome)
}
