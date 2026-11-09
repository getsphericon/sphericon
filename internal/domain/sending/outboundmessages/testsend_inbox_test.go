package outboundmessages_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/ratelimit"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Globex has no verified Sending domain; Jane is its owner.
func globexTestSend(env *testhelper.TestEnv, via senders, to string) (outboundmessages.Result, error) {
	m := outboundmessages.New(
		env.Bus,
		env.Holds,
		via,
		messagetoken.New("test-secret", "http://local", time.Now),
		time.Now,
		outboundmessages.WithSystemSender(env.SystemMail),
		outboundmessages.WithUsers(users.New(env.DB, env.Bus, nil, nil, time.Now)),
	)
	return m.SendTest(context.Background(), asJane(env, fixtures.GlobexID), outboundmessages.TestRequest{
		To:      to,
		Content: outboundmessages.EmailContent{Subject: "[Test] Hi", Body: htmlBody},
	})
}

func TestTestSendThroughASenderThatDeliversNowhereIsCapturedNotRoutedToTheSystemSender(t *testing.T) {
	env := testhelper.Setup(t)
	kept := &keepingSender{}

	res, err := globexTestSend(env, senders{sender: kept}, "someone-else@example.com")
	require.NoError(t, err)

	assert.Equal(t, outboundmessages.Sent, res.Outcome)
	assert.True(t, res.Captured)
	require.Len(t, kept.sent, 1, "the message goes through the Integration")
	assert.Equal(t, "someone-else@example.com", kept.sent[0].To, "to the address named, not only the asker's")
	assert.Empty(t, env.SystemMail.Messages())
}

func TestTestSendWithoutVerifiedDomainKeepsTheSystemSenderForASenderThatDeliversOutside(t *testing.T) {
	cases := map[string]func(*testhelper.TestEnv) senders{
		"an Integration that delivers outside (SMTP, SES)": func(env *testhelper.TestEnv) senders {
			return senders{sender: env.CustomerMail}
		},
		"no Integration": func(*testhelper.TestEnv) senders { return senders{err: messaging.ErrNoProvider} },
	}
	for name, via := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)

			res, err := globexTestSend(env, via(env), fixtures.OwnerJaneEmail)
			require.NoError(t, err)

			assert.Equal(t, outboundmessages.Sent, res.Outcome)
			assert.False(t, res.Captured)
			require.Len(t, env.SystemMail.Messages(), 1)
			assert.Equal(t, env.SystemMail.Messages()[0].From, res.From)
			assert.Empty(t, env.CustomerMail.Messages())

			other, err := globexTestSend(env, via(env), "stranger@example.com")
			require.NoError(t, err)
			assert.Equal(t, outboundmessages.Refused, other.Outcome, "only the asker's own address")
			assert.Equal(t, outboundmessages.RefuseOtherRecipient, other.Reason)
		})
	}
}

// spentBudget is a Test send budget with no slot left for a minute, of a cap of five.
type spentBudget struct{}

func (spentBudget) TakeTestSend(context.Context, int64, int64) (ratelimit.Refusal, error) {
	return ratelimit.Refusal{Wait: time.Minute, Limit: 5}, nil
}

// The System sender's cap is a refusal with a wait, which every surface answers as a rate limit:
// the Test send is not made.
func TestTestSendFromTheSystemSenderPastItsCapIsRefusedWithAWait(t *testing.T) {
	env := testhelper.Setup(t)
	m := outboundmessages.New(env.Bus, env.Holds, senders{err: messaging.ErrNoProvider},
		messagetoken.New("test-secret", "http://local", time.Now), time.Now,
		outboundmessages.WithSystemSender(env.SystemMail),
		outboundmessages.WithUsers(users.New(env.DB, env.Bus, nil, nil, time.Now)),
		outboundmessages.WithTestSendBudget(spentBudget{}))

	_, err := m.SendTest(t.Context(), asJane(env, fixtures.GlobexID), outboundmessages.TestRequest{
		To: fixtures.OwnerJaneEmail, Content: outboundmessages.EmailContent{Subject: "[Test] Hi", Body: htmlBody},
	})

	wait, ok := errors.AsType[*ratelimit.WaitError](err)
	require.True(t, ok, "a refusal with a wait, not %v", err)
	assert.Equal(t, ratelimit.PolicyTestSend, wait.Policy)
	assert.Equal(t, time.Minute, wait.Wait)
	assert.Equal(t, 5, wait.Limit)
	assert.Empty(t, env.SystemMail.Messages())
}

// A Test send asked for through a token (the scope's actor is no User) never goes from the System
// sender: the Workspace's Sending-domain gate holds it.
func TestTestSendOfATokenNeverGoesFromTheSystemSender(t *testing.T) {
	env := testhelper.Setup(t)
	m := outboundmessages.New(env.Bus, env.Holds, senders{err: messaging.ErrNoProvider},
		messagetoken.New("test-secret", "http://local", time.Now), time.Now,
		outboundmessages.WithSystemSender(env.SystemMail),
		outboundmessages.WithUsers(users.New(env.DB, env.Bus, nil, nil, time.Now)))
	token := env.Bus.Act(env.DB.Scoped(fixtures.GlobexID), events.Actor{Kind: events.ActorAPIToken, ID: "1"})

	res, err := m.SendTest(t.Context(), token, outboundmessages.TestRequest{
		To: fixtures.OwnerJaneEmail, Content: outboundmessages.EmailContent{Subject: "[Test] Hi", Body: htmlBody},
	})

	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Held, res.Outcome)
	assert.Empty(t, env.SystemMail.Messages())
}
