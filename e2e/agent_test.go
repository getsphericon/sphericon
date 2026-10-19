//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An agent drafts in the editor's format (ADR 0040): it reads the Brand kit and the schema, copies
// a starter Template, edits its document, lints it and saves a Broadcast draft over MCP. It cannot
// send: a person activates the draft, and the message that then arrives is sanitized, with the
// variables escaped, its links tracked and the unsubscribe footer added.
func TestAgentDraftNeedsAPersonsActivationAndArrivesSanitized(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	w.SaveBrandKit()
	to := w.NewRecipient()
	w.ImportContactNamed(to, `<img src=x onerror=alert(1)>Eve`)
	agent := w.Agent()

	draft := agent.CopyStarter(StarterWelcomeName())
	require.True(t, draft.Edit("Welcome", "Hello"), "the starter has a heading to edit")
	report := agent.Lint(draft)
	assert.Empty(t, report.Errors, "the document renders")
	id := agent.SaveBroadcast(draft)

	// The agent only drafts: it is not offered a send tool, and a direct call is refused.
	assert.False(t, agent.CanSend())
	assert.Contains(t, agent.TrySchedule(id), "mcp:send")
	w.Inbox.RequireNone(Match{To: to})

	w.ActivateBroadcast(id)
	msg := w.Inbox.Wait(Match{To: to})

	assert.Contains(t, msg.HTML, "Hello, &lt;img src=x onerror=alert(1)&gt;Eve", "the edit is in, and the variable is escaped")
	assert.NotContains(t, msg.HTML, "<img src=x")
	assert.NotContains(t, msg.HTML, "<script")
	assert.Contains(t, msg.HTML, env.BaseURL+"/e/c/", "the button's link goes through the click tracker")
	assert.Contains(t, msg.HTML, env.BaseURL+"/e/o/", "the open pixel is added")
	assert.Contains(t, msg.HTML, "unsubscribe</a>", "the unsubscribe footer is added")
	assert.NotEmpty(t, msg.Header("List-Unsubscribe"))
}
