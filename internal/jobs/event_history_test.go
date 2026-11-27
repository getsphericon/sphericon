package jobs_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// aliceCreatedAndTagged are the Events a CSV import publishes for a Contact it writes: its
// creation and a Tag added to it.
func aliceCreatedAndTagged() []events.DomainEvent {
	return []events.DomainEvent{
		&events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Email: fixtures.ContactAliceEmail},
		&events.ContactTagChange{
			Action: systemevent.ContactTagged, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID,
			Email: fixtures.ContactAliceEmail, Tag: "Imported",
		},
	}
}

// An Event an import brings is history (ADR 0138): it is stored, but queues no evaluation against
// the Automations (so it enrolls no one, ends no Enrollment as an exit or goal and wakes no Wait for
// event), none against the Rules and no webhooks job. With "treat them as new sign-ups" chosen, the
// Events still name the job, but they are live; without a cause, they are live as ever.
func TestAnImportsEventsAreHistoryUnlessTreatedAsNewSignUps(t *testing.T) {
	for name, tc := range map[string]struct {
		cause events.Cause
		// reactions is how many jobs each reacting consumer queues for the two Events.
		reactions int
	}{
		"an import":                 {cause: events.Import(7), reactions: 0},
		"an import of new sign-ups": {cause: events.ImportAsNew(7), reactions: 2},
		"a Backfill":                {cause: events.Backfill(), reactions: 0},
		"no cause, as a live Event": {cause: events.Cause{}, reactions: 2},
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)

			for _, ev := range aliceCreatedAndTagged() {
				publishIn(events.WithCause(t.Context(), tc.cause), t, env, ev)
			}

			stored := lo.Filter(env.StoredEvents(t, systemevent.ContactCreated, systemevent.ContactTagged), func(e *ent.Event, _ int) bool {
				return e.WorkspaceID == fixtures.AcmeID
			})
			envelopes := env.QueuedEnvelopes(t, systemevent.ContactCreated, systemevent.ContactTagged)
			automationJobs := env.JobsOf(t, jobkind.EvaluateTrigger.Kind)
			ruleJobs := env.JobsOf(t, jobkind.RuleEvaluate.Kind)
			webhookJobs := env.EventJobs(t, jobs.WebhookEventArgs{}.Kind())

			assert.Len(t, stored, 2, "the Events are stored either way")
			for _, e := range stored {
				assert.Equal(t, tc.reactions == 0, events.WasHistory(e), "the stored Event's flag is the envelope's")
			}
			for _, e := range envelopes {
				assert.Equal(t, tc.cause, e.Cause, "the envelope names what brought the Event")
			}
			assert.Len(t, automationJobs, tc.reactions)
			assert.Len(t, ruleJobs, tc.reactions)
			assert.Len(t, webhookJobs, tc.reactions)
		})
	}
}
