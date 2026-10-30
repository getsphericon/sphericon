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
	w.PublishAutomation(Automation{
		TriggerEvent: "e2e.signed_up",
		Subject:      "Welcome from the automation",
		Body:         HTML("Glad you signed up"),
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

func TestAutomationStepWrittenInTheEditorSendsItsBuiltBody(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	to := w.NewRecipient()
	w.ImportContacts(to)
	w.PublishAuthoredAutomation(AuthoredAutomation{
		TriggerEvent: "e2e.authored",
		Subject:      "Written in the editor",
		Text:         "Built from a document for {{ email }}",
	})

	w.RecordEvent(to, "e2e.authored")

	msg := w.Inbox.Wait(Match{To: to})
	assert.Equal(t, "Written in the editor", msg.Subject)
	assert.Contains(t, msg.HTML, "Built from a document for "+to)
}

func TestAutomationHTTPStepCallsTheHookThenContinues(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	to := w.NewRecipient()
	w.ImportContacts(to)
	hook := w.NewHook(true)
	w.PublishHTTPAutomation(HTTPAutomation{
		TriggerEvent: "e2e.hooked",
		Hook:         hook,
		Body:         `{"email": "{{ email }}"}`,
		Subject:      "After the hook",
		EmailBody:    HTML("The hook was called"),
	})

	w.RecordEvent(to, "e2e.hooked")

	call := hook.Wait()
	assert.JSONEq(t, `{"email": "`+to+`"}`, call.Body)
	// The Enrollment moved past the HTTP step: its email step ran.
	assert.Equal(t, "After the hook", w.Inbox.Wait(Match{To: to}).Subject)
}

// An opened Automation email is an Event that can start another Automation (ADR 0053). The Event
// names the Automation whose email it came from, and that Automation never starts from it.
func TestOpenedAutomationEmailStartsAnotherAutomation(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	to := w.NewRecipient()
	w.ImportContacts(to)
	w.PublishAutomation(Automation{
		TriggerEvent: "e2e.welcomed",
		Subject:      "First email",
		Body:         HTML("Welcome"),
	})
	w.PublishAutomation(Automation{
		TriggerEvent: "email.opened",
		Subject:      "Second email",
		Body:         HTML("Thanks for reading"),
	})

	w.RecordEvent(to, "e2e.welcomed")
	w.OpenMail(w.Inbox.Wait(Match{To: to, Subject: "First email"}))

	second := w.Inbox.Wait(Match{To: to, Subject: "Second email"})

	// The second Automation's own email is opened: that Event starts a third Automation, not the
	// second again.
	w.PublishAutomation(Automation{
		TriggerEvent: "email.opened",
		Subject:      "Third email",
		Body:         HTML("Still reading"),
	})
	w.OpenMail(second)
	w.Inbox.Wait(Match{To: to, Subject: "Third email"})
}
