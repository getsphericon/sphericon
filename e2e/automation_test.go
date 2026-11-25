//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"

	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// A collected Event enrolls its Contact in the Automation it triggers: the ingest publishes it,
// its automations job queues the evaluation, which makes the Enrollment, whose email step sends.
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
		TriggerEvent: systemevent.EmailOpened,
		Subject:      "Second email",
		Body:         HTML("Thanks for reading"),
	})

	w.RecordEvent(to, "e2e.welcomed")
	w.OpenMail(w.Inbox.Wait(Match{To: to, Subject: "First email"}))

	second := w.Inbox.Wait(Match{To: to, Subject: "Second email"})

	// The second Automation's own email is opened: that Event starts a third Automation, not the
	// second again.
	w.PublishAutomation(Automation{
		TriggerEvent: systemevent.EmailOpened,
		Subject:      "Third email",
		Body:         HTML("Still reading"),
	})
	w.OpenMail(second)
	w.Inbox.Wait(Match{To: to, Subject: "Third email"})
}

// A series keyed by order_id runs once per order (ADR 0165): each order's Event enrolls its own
// Enrollment, which sends its own message, and a cancel of one order ends only its series.
func TestAKeyedSeriesSendsOneMessagePerOrderAndACancelEndsOnlyItsOwn(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	to := w.NewRecipient()
	w.ImportContacts(to)
	automationID := w.PublishKeyedAutomation(KeyedAutomation{
		TriggerEvent: "e2e.order_placed",
		Key:          "order_id",
		ExitEvent:    "e2e.order_cancelled",
		Subject:      "Order {{ event.properties.order_id }} confirmed",
	})

	w.RecordEventWith(to, "e2e.order_placed", map[string]string{"order_id": "A-1"})
	w.RecordEventWith(to, "e2e.order_placed", map[string]string{"order_id": "B-2"})

	assert.Equal(t, "Order A-1 confirmed", w.Inbox.Wait(Match{To: to, Subject: "Order A-1 confirmed"}).Subject)
	assert.Equal(t, "Order B-2 confirmed", w.Inbox.Wait(Match{To: to, Subject: "Order B-2 confirmed"}).Subject)

	w.RecordEventWith(to, "e2e.order_cancelled", map[string]string{"order_id": "A-1"})

	want := map[string]externalapi.EnrollmentStatus{"A-1": externalapi.EnrollmentStatusExited, "B-2": externalapi.EnrollmentStatusActive}
	require.Eventually(t, func() bool {
		got := w.EnrollmentsByKey(automationID)
		return got["A-1"] == want["A-1"] && got["B-2"] == want["B-2"]
	}, EmailTimeout, 100*time.Millisecond, "the cancel of A-1 never ended its series alone")
	assert.Equal(t, want, w.EnrollmentsByKey(automationID))
}

// A Branch on a property of the Event that began the Enrollment (ADR 0124, ADR 0180) sends the
// matching path's message to the Contact whose Event matches, the otherwise path's to the other,
// and both paths rejoin: each Contact then receives the email after the Branch.
func TestABranchOnAnEventPropertySendsEachPathsMessageAndThenTheStepAfterIt(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	gold, silver := w.NewRecipient(), w.NewRecipient()
	w.ImportContacts(gold, silver)
	w.PublishBranchAutomation(BranchAutomation{
		TriggerEvent: "e2e.ordered", Key: "plan", Value: "gold",
		PathSubject: "Thanks for going gold", OtherwiseSubject: "Have you seen gold?", AfterSubject: "See you soon",
	})

	w.RecordEventWith(gold, "e2e.ordered", map[string]string{"plan": "gold"})
	w.RecordEventWith(silver, "e2e.ordered", map[string]string{"plan": "silver"})

	w.Inbox.Wait(Match{To: gold, Subject: "Thanks for going gold"})
	w.Inbox.Wait(Match{To: silver, Subject: "Have you seen gold?"})
	w.Inbox.Wait(Match{To: gold, Subject: "See you soon"})
	w.Inbox.Wait(Match{To: silver, Subject: "See you soon"})
	w.Inbox.RequireNone(Match{To: gold, Subject: "Have you seen gold?"})
	w.Inbox.RequireNone(Match{To: silver, Subject: "Thanks for going gold"})
}
