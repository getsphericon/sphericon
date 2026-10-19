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

// Hostile content reaches the Inbox sanitized (ADR 0040): the saved body loses script,
// event handlers and javascript: URLs, and a first name that holds markup is shown as text.
func TestBroadcastWithHostileContentArrivesSanitized(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	to := w.NewRecipient()

	w.ImportContactNamed(to, `<img src=x onerror=alert(1)>Eve`)
	w.SendBroadcast(Broadcast{
		Subject: "Hello {{ first_name }}",
		Body: HTML(`Hi {{ first_name }} <a href="javascript:alert(1)" onclick="steal()">trap</a> `+
			`<a href="https://example.com/ok">fine</a>`) + `<script>alert(1)</script><iframe src="https://evil.example/"></iframe>`,
	})
	msg := w.Inbox.Wait(Match{To: to})

	assert.Equal(t, `Hello <img src=x onerror=alert(1)>Eve`, msg.Subject, "the subject is plain text")
	assert.Contains(t, msg.HTML, "Hi &lt;img src=x onerror=alert(1)&gt;Eve", "markup in a name is shown as text")
	assert.NotContains(t, msg.HTML, "<img src=x")
	assert.NotContains(t, msg.HTML, "<script")
	assert.NotContains(t, msg.HTML, "<iframe")
	assert.NotContains(t, msg.HTML, "javascript:")
	assert.NotContains(t, msg.HTML, "onclick")
	assert.NotContains(t, msg.HTML, "evil.example")
	assert.Contains(t, msg.HTML, "fine</a>", "legitimate links stay")
}

// A Broadcast written in the editor arrives as the message the server built from its document:
// the same path as any Broadcast, tracked and with the unsubscribe footer.
func TestBroadcastWrittenInTheEditorIsDelivered(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	to := w.NewRecipient()

	w.ImportContacts(to)
	w.SendBroadcast(Broadcast{
		Subject: "From the editor",
		Doc: []byte(`{"type":"doc","content":[` +
			`{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"Big news"}]},` +
			`{"type":"paragraph","content":[{"type":"text","text":"Hello {{ email }}"}]},` +
			`{"type":"button","attrs":{"href":"https://example.com/news","alignment":"center"},"content":[{"type":"text","text":"Read more"}]}]}`),
	})
	msg := w.Inbox.Wait(Match{To: to})

	assert.Equal(t, "From the editor", msg.Subject)
	assert.Contains(t, msg.HTML, "Big news")
	assert.Contains(t, msg.HTML, "Hello "+to, "the merge tag is filled for the recipient")
	assert.Contains(t, msg.HTML, "Read more")
	assert.Contains(t, msg.HTML, env.BaseURL+"/e/c/", "the button's link goes through the click tracker")
	assert.NotContains(t, msg.HTML, `href="https://example.com/news"`)
	assert.Contains(t, msg.HTML, env.BaseURL+"/e/o/", "the open pixel is added")
	assert.Contains(t, msg.HTML, "unsubscribe</a>", "the unsubscribe footer is added")
}
