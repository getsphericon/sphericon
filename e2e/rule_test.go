//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCollectedEventTagsTheContactThroughARule(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t)
	clicker, bystander := w.NewRecipient(), w.NewRecipient()
	w.ImportContacts(clicker, bystander)
	w.PublishRule(Rule{TriggerEvent: "e2e.pricing_viewed", Tag: "e2e-interested"})

	// An Event no Rule reacts to first, for a Contact the matching Event never touches.
	w.Visit(bystander, "e2e.something_else")
	w.Visit(clicker, "e2e.pricing_viewed")

	w.RequireTagged(clicker, "e2e-interested")
	// The matching Event's effect has landed, so the earlier Event has long been through the
	// same pipeline: its Contact must carry nothing.
	w.RequireNotTagged(bystander, "e2e-interested")
}

// A Rule is how an Event that no Automation triggers on still enrolls a Contact: the Automation
// here starts from an Event nobody records, so only the Rule's enroll Action reaches the Contact.
func TestCollectedEventMakesARuleTagAndEnrollTheContact(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	visitor, bystander := w.NewRecipient(), w.NewRecipient()
	w.ImportContacts(visitor, bystander)
	automation := w.PublishAutomation(Automation{
		TriggerEvent: "e2e.never_recorded",
		Subject:      "Enrolled by a rule",
		Body:         HTML("A Rule enrolled you"),
	})
	w.PublishRule(Rule{TriggerEvent: "e2e.pricing_viewed", Tag: "e2e-interested", EnrollIn: automation})

	w.Visit(bystander, "e2e.something_else")
	w.Visit(visitor, "e2e.pricing_viewed")

	w.RequireTagged(visitor, "e2e-interested")
	assert.Equal(t, "Enrolled by a rule", w.Inbox.Wait(Match{To: visitor}).Subject)
	w.RequireNotTagged(bystander, "e2e-interested")
	w.Inbox.RequireNone(Match{To: bystander})
}

// A Rule is built in the console with a typed test of a property of the Event that fires it (ADR
// 0181), so only an Event whose property passes tags its Contact; once its Condition is edited and
// published again, the new test decides.
func TestARuleBuiltAndEditedInTheConsoleTestsThePropertiesOfTheEventThatFiresIt(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t)
	pro, free, large, small := w.NewRecipient(), w.NewRecipient(), w.NewRecipient(), w.NewRecipient()
	w.ImportContacts(pro, free, large, small)
	rule := w.PublishRule(Rule{
		TriggerEvent: "e2e.pricing_viewed",
		Tag:          "e2e-qualified",
		Condition: `{"kind":"group","combinator":"and","entries":[
			{"kind":"property","of":{"kind":"trigger"},"key":"plan","test":{"kind":"text","operator":"eq","value":"pro"}}]}`,
	})

	w.VisitWith(free, "e2e.pricing_viewed", `{"plan":"free"}`)
	w.VisitWith(pro, "e2e.pricing_viewed", `{"plan":"pro"}`)

	w.RequireTagged(pro, "e2e-qualified")
	w.RequireNotTagged(free, "e2e-qualified")

	w.EditRuleCondition(rule, `{"kind":"group","combinator":"or","entries":[
		{"kind":"property","of":{"kind":"trigger"},"key":"seats","test":{"kind":"number","operator":"gte","value":10}},
		{"kind":"group","combinator":"and","entries":[
			{"kind":"property","of":{"kind":"trigger"},"key":"seats","test":{"kind":"presence","operator":"notSet"}},
			{"kind":"property","of":{"kind":"trigger"},"key":"plan","test":{"kind":"text","operator":"eq","value":"enterprise"}}]}]}`)

	w.VisitWith(small, "e2e.pricing_viewed", `{"plan":"pro","seats":3}`)
	w.VisitWith(large, "e2e.pricing_viewed", `{"plan":"free","seats":12}`)

	w.RequireTagged(large, "e2e-qualified")
	w.RequireNotTagged(small, "e2e-qualified")
}
