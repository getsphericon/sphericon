package outbound_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
)

const linkHTML = `<p><a href="https://example.com/offer">Offer</a></p>`

// sendFrom sends one tracked marketing message from a Sending domain and returns it as built.
func sendFrom(t *testing.T, env *testhelper.TestEnv, m *outbound.Module, key, domain string) messaging.EmailMessage {
	t.Helper()
	res, err := m.Send(context.Background(), env.DB.Scoped(fixtures.UmbrellaID), outbound.Request{
		Kind:        outboundmessage.KindBroadcast,
		Key:         key,
		Destination: "reader@example.com",
		Source:      eligibility.SourceBroadcasts,
		Subject:     "News",
		Body:        linkHTML,
		FromEmail:   "news@" + domain,
		TrackID:     1,
	})
	require.NoError(t, err)
	require.Equal(t, outbound.Sent, res.Outcome)
	msgs := env.CustomerMail.Messages()
	return msgs[len(msgs)-1]
}

func tracked(env *testhelper.TestEnv, opts ...outbound.Option) *outbound.Module {
	return outbound.New(env.Bus, senders{sender: env.CustomerMail}, tracking.New("test-secret", "http://local"), opts...)
}

func TestServingTrackingDomainCarriesEveryLink(t *testing.T) {
	env := testhelper.Setup(t)
	msg := sendFrom(t, env, tracked(env, outbound.WithTrackingDomains()), "th:serving", fixtures.SendingDomainTrackingServingDomain)

	host := "https://track." + fixtures.SendingDomainTrackingServingDomain
	assert.Contains(t, msg.HTML, host+"/e/c/", "click link")
	assert.Contains(t, msg.HTML, host+"/e/o/", "open pixel")
	assert.Contains(t, msg.HTML, host+"/e/u/", "unsubscribe footer")
	assert.NotContains(t, msg.HTML, "http://local")
	assert.Contains(t, msg.ListUnsubscribeURL, host+"/e/u/")
}

func TestNonServingOrDisabledTrackingUsesInstanceHost(t *testing.T) {
	env := testhelper.Setup(t)
	cases := []struct {
		name   string
		domain string
		opts   []outbound.Option
	}{
		{"dns verified", fixtures.SendingDomainTrackingDNSVerifiedDomain, []outbound.Option{outbound.WithTrackingDomains()}},
		{"feature off", fixtures.SendingDomainTrackingServingDomain, nil},
	}
	for _, c := range cases {
		msg := sendFrom(t, env, tracked(env, c.opts...), "th:"+c.name, c.domain)
		assert.Contains(t, msg.HTML, "http://local/e/c/", c.name)
		assert.Contains(t, msg.HTML, "http://local/e/o/", c.name)
		assert.Contains(t, msg.HTML, "http://local/e/u/", c.name)
		assert.NotContains(t, msg.HTML, "track.", c.name)
		assert.Contains(t, msg.ListUnsubscribeURL, "http://local/e/u/", c.name)
	}
}

func TestEachSendingDomainGetsItsOwnHost(t *testing.T) {
	env := testhelper.Setup(t)
	m := tracked(env, outbound.WithTrackingDomains())
	serving := sendFrom(t, env, m, "th:a", fixtures.SendingDomainTrackingServingDomain)
	other := sendFrom(t, env, m, "th:b", fixtures.SendingDomainTrackingDNSVerifiedDomain)

	assert.Contains(t, serving.ListUnsubscribeURL, "https://track."+fixtures.SendingDomainTrackingServingDomain+"/")
	assert.Contains(t, other.ListUnsubscribeURL, "http://local/")
}
