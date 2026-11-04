//go:build e2e

package e2e

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A tracked click reaches the Workspace's Webhook endpoint: the tracker publishes email.clicked,
// whose webhooks job queues the delivery the receiver gets, with the public body.
func TestATrackedClickReachesAWebhook(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	to := w.NewRecipient()
	w.ImportContacts(to)
	hook := w.SubscribeWebhook("email.clicked")
	w.SendBroadcast(Broadcast{
		Subject: "Clicked " + uniq(),
		Body:    HTML(`See the <a href="https://example.com/offer">offer</a>`),
	})
	msg := w.Inbox.Wait(Match{To: to})

	w.ClickLink(msg)

	call := hook.Wait()
	var body struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Subject string `json:"subject"`
		Data    struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(call.Body), &body))
	assert.NotEmpty(t, body.ID)
	assert.Equal(t, "email.clicked", body.Type)
	assert.Equal(t, to, body.Subject)
	assert.Equal(t, "https://example.com/offer", body.Data.URL)
}
