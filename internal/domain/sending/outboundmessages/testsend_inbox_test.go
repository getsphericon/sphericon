package outboundmessages_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
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
	)
	return m.SendTest(context.Background(), env.DB.Scoped(fixtures.GlobexID), outboundmessages.TestRequest{
		To:      to,
		Content: outboundmessages.EmailContent{Subject: "[Test] Hi", Body: htmlBody},
		Asker:   &outboundmessages.Asker{UserID: fixtures.OwnerJaneID, Email: fixtures.OwnerJaneEmail},
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
