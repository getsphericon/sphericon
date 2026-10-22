//go:build e2e

package e2e

import "testing"

func TestCollectedEventTagsTheContactThroughARule(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t)
	clicker, bystander := w.NewRecipient(), w.NewRecipient()
	w.ImportContacts(clicker, bystander)
	w.ActivateRule(Rule{TriggerEvent: "e2e.pricing_viewed", Tag: "e2e-interested"})

	// An Event no Rule reacts to first, for a Contact the matching Event never touches.
	w.Visit(bystander, "e2e.something_else")
	w.Visit(clicker, "e2e.pricing_viewed")

	w.RequireTagged(clicker, "e2e-interested")
	// The matching Event's effect has landed, so the earlier Event has long been through the
	// same pipeline: its Contact must carry nothing.
	w.RequireNotTagged(bystander, "e2e-interested")
}
