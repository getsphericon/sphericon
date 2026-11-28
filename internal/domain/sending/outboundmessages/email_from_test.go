package outboundmessages_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// configuredFrom is the sender of an Integration whose configured From is its own address.
type configuredFrom struct {
	*testhelper.CapturingSender

	address string
}

func (s configuredFrom) DefaultFrom() (address, name string) { return s.address, "" }

// withSenders is newModule with the Integrations' senders built by ss.
func withSenders(env *testhelper.TestEnv, ss senders) *outboundmessages.Module {
	return outboundmessages.New(
		env.Bus,
		testhelper.Module[*holds.Holds](env),
		ss,
		messagetoken.New("test-secret", "http://local", time.Now), env.OutboundModules(env.Bus, time.Now),
		outboundmessages.Deps{},
		outboundmessages.Config{},
		time.Now,
	)
}

// The From the email lint reads as a message's sender (ADR 0188): the message's own, else the
// configured From of the Integration it goes through.
func TestEmailFromIsTheMessagesOwnElseItsIntegrations(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := withSenders(env, senders{sender: configuredFrom{CapturingSender: env.CustomerMail, address: "no-reply@acme.com"}})

	got, err := m.EmailFrom(t.Context(), s, nil, "")
	require.NoError(t, err)
	assert.Equal(t, "no-reply@acme.com", got, "the default Integration's configured From")

	got, err = m.EmailFrom(t.Context(), s, nil, "news@acme.com")
	require.NoError(t, err)
	assert.Equal(t, "news@acme.com", got, "the message's own From wins")
}

// An Integration whose sender cannot be built sends nothing: it has no From to check, and the lint
// that asks never fails the save it runs in.
func TestEmailFromOfAnUnbuildableIntegrationIsNone(t *testing.T) {
	env := testhelper.Setup(t)
	m := withSenders(env, senders{err: errors.New("the configuration does not decrypt")})

	got, err := m.EmailFrom(t.Context(), env.DB.Scoped(fixtures.AcmeID), nil, "")

	require.NoError(t, err)
	assert.Empty(t, got)
}
