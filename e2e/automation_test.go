//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRecordedEventTriggersAutomationEmail(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	matching, other := w.NewRecipient(), w.NewRecipient()
	w.ImportContacts(matching, other)
	w.ActivateAutomation(Automation{
		TriggerEvent: "e2e.signed_up",
		Subject:      "Welcome from the automation",
		Body:         MJML("Glad you signed up"),
	})

	// A non-matching event first, for a Contact the matching event never touches.
	w.RecordEvent(other, "e2e.something_else")
	w.RecordEvent(matching, "e2e.signed_up")

	msg := w.Inbox.Wait(Match{To: matching})
	assert.Equal(t, w.FromEmail, msg.From.Address)
	assert.Equal(t, "Welcome from the automation", msg.Subject)
	assert.Contains(t, msg.HTML, "Glad you signed up")

	// The matching event's email has arrived, so the earlier non-matching event has
	// long been through the same pipeline: its Contact must have received nothing.
	w.Inbox.RequireNone(Match{To: other})
}

func TestAutomationHTTPStepCallsTheHookThenContinues(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	to := w.NewRecipient()
	w.ImportContacts(to)
	hook := w.NewHook(true)
	w.ActivateHTTPAutomation(HTTPAutomation{
		TriggerEvent: "e2e.hooked",
		Hook:         hook,
		Body:         `{"email": "{{ email }}"}`,
		Subject:      "After the hook",
		EmailBody:    MJML("The hook was called"),
	})

	w.RecordEvent(to, "e2e.hooked")

	call := hook.Wait()
	assert.JSONEq(t, `{"email": "`+to+`"}`, call.Body)
	// The Enrollment moved past the HTTP step: its email step ran.
	assert.Equal(t, "After the hook", w.Inbox.Wait(Match{To: to}).Subject)
}
