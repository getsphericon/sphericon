package events_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
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
		&events.ContactTagChange{Action: events.NameContactTagged, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Tag: "vip"},
		&events.ContactTagChange{Action: events.NameContactUntagged, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Tag: "vip"},
		&events.ContactFieldChanged{WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Key: "plan", Value: "pro"},
		enrollment(events.NameAutomationEnrolled),
		enrollment(events.NameAutomationCompleted),
		enrollment(events.NameAutomationGoalReached),
		enrollment(events.NameAutomationExited),
	}
	actions := make([]string, 0, len(changes))
	for _, ev := range changes {
		require.NoError(t, env.Publisher().Publish(ctx, ev))
		actions = append(actions, ev.EventName())
	}
	envelopes := env.OutboxEnvelopes(t, actions...)
	for _, envelope := range envelopes {
		require.NoError(t, events.Persist(ctx, env.DB, envelope))
	}
	stored := func() []string {
		names, err := env.DB.Event.Query().Where(event.ActionIn(actions...)).Select(event.FieldAction).Strings(ctx)
		require.NoError(t, err)
		return names
	}
	before := stored()

	_, err := events.PruneEvents(ctx, env.DB, time.Nanosecond, 100)
	require.NoError(t, err)
	after := stored()

	assert.ElementsMatch(t, actions, before, "each is stored first")
	assert.Empty(t, after)
}
