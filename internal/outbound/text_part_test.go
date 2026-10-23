package outbound_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The text part of a send is derived from its final HTML (ADR 0056): tracked links, the unsubscribe
// link and the postal address are in it, the pixel, the hidden preheader and the CSS are not.
const textPartBody = `<style>p{color:#123456}</style>` +
	`<div style="display:none;max-height:0;overflow:hidden">Hidden preview words</div>` +
	`<p>Hi {{ name }}</p><p><a href="https://example.com/offer">Offer</a></p>`

// sendUmbrella sends one message from Umbrella (the Workspace with a postal address) and
// returns the message that reached the provider.
func sendUmbrella(t *testing.T, env *testhelper.TestEnv, req outbound.Request) messaging.EmailMessage {
	t.Helper()
	req.Destination = "reader@example.com"
	req.Subject = "News"
	req.Body = textPartBody
	req.Variables = map[string]any{"name": "Grace"}
	req.FromEmail = "news@" + fixtures.SendingDomainTrackingServingDomain
	res, err := newModule(env).Send(context.Background(), env.DB.Scoped(fixtures.UmbrellaID), req)
	require.NoError(t, err)
	require.Equal(t, outbound.Sent, res.Outcome)
	msgs := env.CustomerMail.Messages()
	return msgs[len(msgs)-1]
}

func marketingRequest(key string, track bool) outbound.Request {
	return outbound.Request{
		Kind:   outboundmessage.KindBroadcast,
		Key:    key,
		Source: eligibility.SourceBroadcasts,
		Track:  track,
	}
}

func requireCleanText(t *testing.T, text string) {
	t.Helper()
	assert.Contains(t, text, "Hi Grace", "merge tags are resolved")
	assert.NotContains(t, text, "Hidden preview words", "the preheader is hidden from readers")
	assert.NotContains(t, text, "color:#123456", "no CSS")
	assert.NotContains(t, text, "/e/o/", "no open pixel")
}

func TestTrackedMarketingTextPartCarriesTrackedLinksUnsubscribeAndAddress(t *testing.T) {
	env := testhelper.Setup(t)
	msg := sendUmbrella(t, env, marketingRequest("tp:tracked", true))

	requireCleanText(t, msg.Text)
	assert.Contains(t, msg.Text, "Offer")
	assert.Contains(t, msg.Text, "http://local/e/c/", "the tracked URL")
	assert.NotContains(t, msg.Text, "https://example.com/offer", "not the raw URL")
	assert.Equal(t, 1, strings.Count(msg.Text, "http://local/e/u/"), "the unsubscribe link, once")
	assert.Contains(t, msg.Text, "1 Umbrella Plaza")
	assert.Contains(t, msg.Text, "Raccoon City")
	assert.Equal(t, 1, strings.Count(msg.Text, "Raccoon City"), "the postal address, once")
}

func TestUntrackedMarketingTextPartCarriesUnsubscribeAndAddress(t *testing.T) {
	env := testhelper.Setup(t)
	msg := sendUmbrella(t, env, marketingRequest("tp:untracked", false))

	requireCleanText(t, msg.Text)
	assert.Contains(t, msg.Text, "https://example.com/offer", "untracked links stay as written")
	assert.Equal(t, 1, strings.Count(msg.Text, "http://local/e/u/"), "the unsubscribe link, once")
	assert.Equal(t, 1, strings.Count(msg.Text, "Raccoon City"), "the postal address, once")
}
