package outbound_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/dkimstate"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/getsphericon/sphericon/internal/trackingdomains"
)

const linkHTML = `<p><a href="https://example.com/offer">Offer</a></p>`

// sendFrom sends one tracked marketing message of the Workspace from its Sending domain and
// returns it as built.
func sendFrom(t *testing.T, env *testhelper.TestEnv, m *outbound.Module, ws int64, key, domain string) messaging.EmailMessage {
	t.Helper()
	res, err := m.Send(context.Background(), env.DB.Scoped(ws), outbound.Request{
		Kind:        outboundmessage.KindBroadcast,
		Key:         key,
		Destination: "reader@example.com",
		Source:      eligibility.SourceBroadcasts,
		Content:     outbound.EmailContent{Subject: "News", Body: linkHTML, FromEmail: "news@" + domain},
		Track:       true,
	})
	require.NoError(t, err)
	require.Equal(t, outbound.Sent, res.Outcome)
	msgs := env.CustomerMail.Messages()
	return msgs[len(msgs)-1]
}

func tracked(env *testhelper.TestEnv, opts ...outbound.Option) *outbound.Module {
	return outbound.New(env.Bus, env.Holds, senders{sender: env.CustomerMail}, tracking.New("test-secret", "http://local"), opts...)
}

// trackingDomains is the option the composition root passes while the Instance offers Tracking
// domains.
func trackingDomains(env *testhelper.TestEnv) outbound.Option {
	return outbound.WithTrackingDomains(trackingdomains.New(env.DB, env.Bus, nil, trackingdomains.Config{}))
}

// Every link of a Workspace whose current Tracking domain serves is on it, whatever the
// From domain (ADR 0071).
func TestServingTrackingDomainCarriesEveryLink(t *testing.T) {
	env := testhelper.Setup(t)
	msg := sendFrom(t, env, tracked(env, trackingDomains(env)), fixtures.UmbrellaID, "th:serving", fixtures.SendingDomainUmbrellaDomain)

	host := "https://" + fixtures.TrackingDomainServingHost
	assert.Contains(t, msg.HTML, host+"/e/c/", "click link")
	assert.Contains(t, msg.HTML, host+"/e/o/", "open pixel")
	assert.Contains(t, msg.HTML, host+"/e/u/", "unsubscribe footer")
	assert.NotContains(t, msg.HTML, "http://local")
	assert.Contains(t, msg.ListUnsubscribeURL, host+"/e/u/")
}

func TestNonServingOrDisabledTrackingUsesInstanceHost(t *testing.T) {
	env := testhelper.Setup(t)
	cases := []struct {
		name string
		ws   int64
		opts []outbound.Option
	}{
		{"dns verified", fixtures.InitechID, []outbound.Option{trackingDomains(env)}},
		{"feature off", fixtures.UmbrellaID, nil},
	}
	for _, c := range cases {
		domain, err := env.DB.SendingDomain.Query().Where(sendingdomain.WorkspaceID(c.ws), dkimstate.Verified()).First(t.Context())
		require.NoError(t, err, c.name)
		msg := sendFrom(t, env, tracked(env, c.opts...), c.ws, "th:"+c.name, domain.Domain)
		assert.Contains(t, msg.HTML, "http://local/e/c/", c.name)
		assert.Contains(t, msg.HTML, "http://local/e/o/", c.name)
		assert.Contains(t, msg.HTML, "http://local/e/u/", c.name)
		assert.Contains(t, msg.ListUnsubscribeURL, "http://local/e/u/", c.name)
	}
}

// A tracking host that cannot be read says nothing about the message: the send is retried, not recorded
// as Failed for good.
func TestTrackingHostLookupErrorIsRetriedNotFailed(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := tracked(env, trackingDomains(env))
	scoped := env.DB.Scoped(fixtures.UmbrellaID)
	lookup := errors.New("connection reset")

	// The host lookup is the last Tracking domain query of a tracked send: count them on a first
	// send, then fail that last one.
	var count, failAt int
	env.DB.TrackingDomain.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
			count++
			if count == failAt {
				return nil, lookup
			}
			return next.Query(ctx, q)
		})
	}))
	send := func(key string) (outbound.Result, error) {
		return m.Send(ctx, scoped, outbound.Request{
			Kind:        outboundmessage.KindBroadcast,
			Key:         key,
			Destination: "reader@example.com",
			Source:      eligibility.SourceBroadcasts,
			Content:     outbound.EmailContent{Subject: "News", Body: linkHTML, FromEmail: "news@" + fixtures.SendingDomainUmbrellaDomain},
			Track:       true,
		})
	}
	first, err := send("th:count")
	require.NoError(t, err)
	require.Equal(t, outbound.Sent, first.Outcome)
	failAt, count = count, 0

	res, err := send("th:lookup")
	require.ErrorIs(t, err, lookup)
	require.ErrorContains(t, err, "tracking host", "it is the host lookup that failed")
	assert.NotEqual(t, outbound.Failed, res.Outcome)
	row := env.DB.OutboundMessage.Query().Where(outboundmessage.IdempotencyKeyContains("th:lookup")).OnlyX(ctx)
	assert.Equal(t, outboundmessage.StatusPending, row.Status, "the claim stays open for the retry")

	failAt = 0
	retried, err := send("th:lookup")
	require.NoError(t, err)
	assert.Equal(t, outbound.Sent, retried.Outcome)
	assert.Len(t, env.CustomerMail.Messages(), 2)
}
