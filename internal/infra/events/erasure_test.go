package events_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// deletedOnErasure are the Events a Contact's Erasure deletes rather than keeps anonymous: the
// customer's own, the consent proof (a tracking grant's and a return to everything too), the person's own words and the
// Contact's own changes.
var deletedOnErasure = []string{
	events.NameCollected, systemevent.MarketingConfirmed, systemevent.SmsConfirmed,
	systemevent.TelegramReceived, systemevent.SmsReceived, systemevent.ContactErased,
	systemevent.ContactTagged, systemevent.ContactUntagged, systemevent.ContactFieldChanged,
	systemevent.AutomationEnrolled, systemevent.AutomationCompleted, systemevent.AutomationGoalReached,
	systemevent.AutomationExited,
	systemevent.EmailResubscribed,
	systemevent.EmailEngagementTrackingGranted, systemevent.SmsEngagementTrackingGranted, systemevent.TelegramEngagementTrackingGranted,
}

// Every event with an Event row says what a Contact's Erasure does with it, so a new one is kept
// anonymous or deleted on purpose, never by a list it was left out of.
func TestEveryEventSaysWhetherErasureKeepsIt(t *testing.T) {
	stored := lo.Filter(events.Names(), func(name string, _ int) bool { return !events.IsUnprojected(name) })
	kept := lo.Filter(stored, func(name string, _ int) bool { return events.KeptOnErasure(name) })

	assert.ElementsMatch(t, stored, append(kept, deletedOnErasure...),
		"each event with an Event row is keptOnErasure or listed in deletedOnErasure, never both")
	assert.False(t, events.KeptOnErasure("purchase"), "a customer's own action is deleted")
}
