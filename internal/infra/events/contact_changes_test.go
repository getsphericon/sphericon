package events_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The System events of a Contact's Tags, Custom fields and Enrollments are analytical: Retention
// deletes them by age (#897, ADR 0019).
func TestContactChangeEventsExpireByAge(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	enrollment := func(action string) events.DomainEvent {
		return &events.EnrollmentChange{
			Action: action, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, AutomationID: fixtures.AutomationJobsGreetingID,
		}
	}
	changes := []events.DomainEvent{
		&events.ContactTagChange{Action: systemevent.ContactTagged, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Tag: "vip"},
		&events.ContactTagChange{Action: systemevent.ContactUntagged, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Tag: "vip"},
		&events.ContactFieldChanged{WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Key: "plan", Value: "pro"},
		enrollment(systemevent.AutomationEnrolled),
		enrollment(systemevent.AutomationCompleted),
		enrollment(systemevent.AutomationGoalReached),
		enrollment(systemevent.AutomationExited),
	}
	actions := make([]string, 0, len(changes))
	for _, ev := range changes {
		require.NoError(t, env.Publisher().Publish(ctx, ev))
		actions = append(actions, ev.EventName())
	}
	stored := func() []string {
		names, err := env.DB.Event.Query().Where(event.ActionIn(actions...)).Select(event.FieldAction).Strings(ctx)
		require.NoError(t, err)
		return names
	}
	before := stored()

	_, err := events.PruneEvents(ctx, env.DB, time.Nanosecond, 100, time.Now())
	require.NoError(t, err)
	after := stored()

	assert.ElementsMatch(t, actions, before, "each is stored first")
	assert.Empty(t, after)
}
