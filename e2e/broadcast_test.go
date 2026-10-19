//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBroadcastToImportedContactsIsDelivered(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	to := w.NewRecipient()

	w.ImportContacts(to)
	w.SendBroadcast(Broadcast{
		Subject: "Hello from e2e",
		Body:    HTML(`Welcome aboard, friend <a href="https://example.com/start">Get started</a>`),
	})
	msg := w.Inbox.Wait(Match{To: to})

	assert.Equal(t, w.FromEmail, msg.From.Address)
	assert.Equal(t, w.FromName, msg.From.Name)
	assert.Equal(t, "Hello from e2e", msg.Subject)
	assert.Contains(t, msg.HTML, "Welcome aboard, friend")
	assert.Contains(t, msg.Text, "Welcome aboard, friend")

	assert.Contains(t, msg.HTML, env.BaseURL+"/e/c/", "links go through the click tracker")
	assert.NotContains(t, msg.HTML, `href="https://example.com/start"`, "the destination is not linked directly")
	assert.Contains(t, msg.HTML, env.BaseURL+"/e/o/", "the open pixel is added")
	assert.Contains(t, msg.HTML, "unsubscribe</a>", "the unsubscribe footer is added")

	unsub := msg.Header("List-Unsubscribe")
	require.NotEmpty(t, unsub, "a Broadcast carries List-Unsubscribe")
	assert.Contains(t, unsub, env.BaseURL, "the unsubscribe link points at the server under test")
	assert.Equal(t, "List-Unsubscribe=One-Click", msg.Header("List-Unsubscribe-Post"))
}
