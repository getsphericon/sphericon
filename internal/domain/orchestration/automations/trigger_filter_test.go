package automations_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// propertyEq is the filter whose one test is that the Event's property key is value.
func propertyEq(t *testing.T, key, value string) *conditiontree.EventFilter {
	t.Helper()
	leaf, err := json.Marshal(map[string]any{
		"kind": "property", "key": key, "test": map[string]any{"kind": "text", "operator": "eq", "value": value},
	})
	require.NoError(t, err)
	return &conditiontree.EventFilter{Kind: conditiontree.KindGroup, Combinator: conditiontree.And, Entries: []json.RawMessage{leaf}}
}

// liveOn creates an Automation of no Steps whose Trigger is the matcher and publishes it, so it
// enrolls (and completes at once). It returns its id.
func liveOn(t *testing.T, env *testhelper.TestEnv, trigger eventmatcher.Matcher) int64 {
	t.Helper()
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	a, err := m.Create(ctx, s, automations.CreateInput{Name: "Filtered " + trigger.Action, Trigger: trigger})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))
	require.NoError(t, err)
	return a.ID
}

// handStored publishes the event of the action, which stores its row, and hands that stored Event
// to the Automations as its trigger job does.
func handStored(t *testing.T, env *testhelper.TestEnv, contactID int64, action string, ev events.DomainEvent) {
	t.Helper()
	require.NoError(t, env.Publisher().Publish(context.Background(), ev))
	stored := env.StoredEvents(t, action)
	require.NotEmpty(t, stored)
	last := stored[len(stored)-1]
	in := automations.OnEventInput{ContactID: contactID, Event: automations.Event{Action: last.Action, SourceID: *last.SourceID, OccurredAt: last.OccurredAt}}
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(context.Background(), env.DB.Scoped(fixtures.AcmeID), in))
}

// tagged is contact.tagged of the Tag for the Contact.
func tagged(contactID int64, tag string) *events.ContactTagChange {
	return &events.ContactTagChange{Action: systemevent.ContactTagged, WorkspaceID: fixtures.AcmeID, ContactID: contactID, Tag: tag}
}

// A Trigger on contact.tagged where tag is trial enrolls on that Tag only: another Tag enrolls no
// one and is not a skipped entry (ADR 0133).
func TestATriggerFilterOnTheTagEnrollsOnThatTagOnly(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	_, err := tags.New().EnsureIn(context.Background(), s, "trial")
	require.NoError(t, err)
	id := liveOn(t, env, eventmatcher.Matcher{Action: systemevent.ContactTagged, Filter: propertyEq(t, "tag", "trial")})
	onTrial, onVip := enrollable(t, s, 0), enrollable(t, s, 1)

	handStored(t, env, onVip.ID, systemevent.ContactTagged, tagged(onVip.ID, "vip"))
	handStored(t, env, onTrial.ID, systemevent.ContactTagged, tagged(onTrial.ID, "trial"))

	assert.Empty(t, runsOf(t, env, id, onVip.ID), "another Tag enrolls no one")
	assert.Empty(t, skippedOf(t, env, id, onVip.ID), "a filter miss is no skipped entry")
	assert.Len(t, runsOf(t, env, id, onTrial.ID), 1)
}

// A Trigger on order.completed where status is paid enrolls on a paid order and not on a failed
// one.
func TestATriggerFilterOnAPropertyEnrollsOnAMatchingEventOnly(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	id := liveOn(t, env, eventmatcher.Matcher{Action: "order.completed", Filter: propertyEq(t, "status", "paid")})
	paid, failed := enrollable(t, s, 0), enrollable(t, s, 1)
	order := func(contactID int64, status string) *events.CollectedEvent {
		return &events.CollectedEvent{WorkspaceID: fixtures.AcmeID, ContactID: contactID, Action: "order.completed",
			Properties: map[string]any{"status": status}, OccurredAt: time.Now()}
	}

	handStored(t, env, failed.ID, "order.completed", order(failed.ID, "failed"))
	handStored(t, env, paid.ID, "order.completed", order(paid.ID, "paid"))

	assert.Empty(t, runsOf(t, env, id, failed.ID))
	assert.Empty(t, skippedOf(t, env, id, failed.ID))
	assert.Len(t, runsOf(t, env, id, paid.ID), 1)
}
