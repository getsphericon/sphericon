package events_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// msgCausedBy is msgFor for an event an Action of a Rule published: its envelope carries the marker.
func msgCausedBy(t *testing.T, ev events.DomainEvent, cause string) *message.Message {
	t.Helper()
	data, err := json.Marshal(ev)
	require.NoError(t, err)
	body, err := json.Marshal(events.Envelope{ID: "evt_caused", Name: ev.EventName(), Version: ev.EventVersion(), WorkspaceID: ev.Workspace(), Data: data, CausedBy: cause})
	require.NoError(t, err)
	return message.NewMessage(watermill.NewUUID(), body)
}

// A recorded Event for a Contact is queued for evaluation with what a Firing is unique on (the
// envelope id, as no DedupKey is set) and the properties it carries.
func TestRulesConsumerQueuesTheEventForEvaluation(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, events.RulesConsumer(env.DB, queueingJobs(t, env))(msgFor(t, &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, SubjectID: "alice", Action: "rule_click",
		Properties: map[string]any{"url": "https://example.com/a"},
	})))

	queued := env.JobsOf(t, jobkind.RuleEvaluate.Kind)
	require.Len(t, queued, 1)
	assert.EqualValues(t, fixtures.AcmeID, queued[0]["workspace_id"])
	assert.EqualValues(t, fixtures.ContactAliceID, queued[0]["contact_id"])
	assert.Equal(t, "rule_click", queued[0]["action"])
	assert.Equal(t, "evt_test", queued[0]["source_id"])
	assert.Equal(t, map[string]any{"url": "https://example.com/a"}, queued[0]["properties"])
}

// The Event that fires a Rule is stored before its evaluation is queued, so a Condition that counts
// the Contact's Events of the trigger's action counts it too, whichever consumer runs first. The
// persist consumer's later write of the same envelope adds no second row.
func TestRulesConsumerStoresTheEventBeforeQueueingIt(t *testing.T) {
	env := testhelper.Setup(t)
	msg := msgFor(t, &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, SubjectID: "alice", Action: "pricing_viewed",
	})

	require.NoError(t, events.RulesConsumer(env.DB, queueingJobs(t, env))(msg))
	stored, err := env.DB.Event.Query().Where(event.SourceID("evt_test")).Only(t.Context())
	require.NoError(t, err)
	queued := env.JobsOf(t, jobkind.RuleEvaluate.Kind)
	var envelope events.Envelope
	require.NoError(t, json.Unmarshal(msg.Payload, &envelope))
	require.NoError(t, events.Persist(t.Context(), env.DB, envelope))
	rows, err := env.DB.Event.Query().Where(event.SourceID("evt_test")).Count(t.Context())
	require.NoError(t, err)

	assert.Equal(t, "pricing_viewed", stored.Action)
	assert.EqualValues(t, fixtures.ContactAliceID, *stored.ContactID)
	assert.Len(t, queued, 1)
	assert.Equal(t, 1, rows)
}

func TestRulesConsumerSkipsAnEventWithNoContact(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, events.RulesConsumer(env.DB, queueingJobs(t, env))(msgFor(t, &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID, SubjectID: "visitor:x", Action: "rule_click",
	})))

	assert.Empty(t, env.JobsOf(t, jobkind.RuleEvaluate.Kind))
}

func TestRulesConsumerSkipsAnEventAnActionPublished(t *testing.T) {
	env := testhelper.Setup(t)
	ev := &events.CollectedEvent{WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, SubjectID: "alice", Action: "rule_click"}

	require.NoError(t, events.RulesConsumer(env.DB, queueingJobs(t, env))(msgCausedBy(t, ev, "rule:1")))

	assert.Empty(t, env.JobsOf(t, jobkind.RuleEvaluate.Kind))
}

func TestRulesConsumerSkipsHistoryAndAuditEntries(t *testing.T) {
	env := testhelper.Setup(t)
	consume := events.RulesConsumer(env.DB, queueingJobs(t, env))

	history := &events.CollectedEvent{WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, SubjectID: "alice", Action: "rule_click", Backfill: true}
	require.NoError(t, consume(msgFor(t, history)))
	require.NoError(t, consume(msgFor(t, entryBy(events.Actor{Kind: events.ActorUser}, "membership.update"))))

	assert.Empty(t, env.JobsOf(t, jobkind.RuleEvaluate.Kind))
}

// An Event heavier than RulePropertiesBound is queued without its properties.
func TestRulesConsumerQueuesAnOversizedEventWithoutItsProperties(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, events.RulesConsumer(env.DB, queueingJobs(t, env))(msgFor(t, &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, SubjectID: "alice", Action: "rule_click",
		Properties: map[string]any{"pad": strings.Repeat("x", events.RulePropertiesBound)},
	})))

	queued := env.JobsOf(t, jobkind.RuleEvaluate.Kind)
	require.Len(t, queued, 1)
	assert.NotContains(t, queued[0], "properties")
}

func TestRulePropertiesKeepsWhatFitsTheBoundAndDropsWhatDoesNot(t *testing.T) {
	small := map[string]any{"url": "https://x.test"}
	large := map[string]any{"pad": strings.Repeat("x", events.RulePropertiesBound)}

	for name, tc := range map[string]struct{ in, want map[string]any }{
		"none":      {in: nil, want: nil},
		"fits":      {in: small, want: small},
		"too large": {in: large, want: nil},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, events.RuleProperties(tc.in))
		})
	}
}

// A machine open or click is recorded but triggers no Rule (ADR 0053, ADR 0054).
func TestRulesConsumerSkipsMachineEngagement(t *testing.T) {
	env := testhelper.Setup(t)
	react := events.RulesConsumer(env.DB, queueingJobs(t, env))

	for _, action := range []string{events.NameEmailOpened, events.NameEmailClicked} {
		require.NoError(t, react(msgFor(t, &events.EmailEngagement{
			Action: action, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID,
			Email: fixtures.ContactAliceEmail, Machine: true,
		})))
	}
	assert.Empty(t, env.JobsOf(t, jobkind.RuleEvaluate.Kind))

	require.NoError(t, react(msgFor(t, &events.EmailEngagement{
		Action: events.NameEmailOpened, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID,
		Email: fixtures.ContactAliceEmail,
	})))
	assert.Len(t, env.JobsOf(t, jobkind.RuleEvaluate.Kind), 1, "a person's open still triggers")
}
