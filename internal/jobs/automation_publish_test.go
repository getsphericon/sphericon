package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// recordingSenders resolves every Integration to fs and records which one each send went through.
func recordingSenders(fs *fakeSender, through *[]int64) *SendersMock {
	return &SendersMock{EmailSenderFunc: func(_ context.Context, _ *ent.Scoped, integ *ent.Integration) (messaging.EmailSender, error) {
		*through = append(*through, integ.ID)
		return fs, nil
	}}
}

func subjectsOf(fs *fakeSender) []string {
	return lo.Map(fs.sent, func(m messaging.EmailMessage, _ int) string { return m.Subject })
}

// editGreeting rewrites the greeting Automation's Trigger, Steps and Integration through /api.
func editGreeting(t *testing.T, c *externalapi.Client) {
	t.Helper()
	res, err := c.AutomationsUpdate(t.Context(), &externalapi.UpdateAutomationInput{
		TriggerEvent: externalapi.NewOptString("job_hijacked"),
		Steps: []externalapi.AutomationStep{{
			Type:    externalapi.AutomationStepTypeEmail,
			Subject: externalapi.NewOptString("Hijacked"),
			Body:    externalapi.NewOptString("<p>Hijacked</p>"),
		}},
		IntegrationId: externalapi.NewOptNilEntityId(externalapi.EntityId(fixtures.IntegrationAcmeMailgunID)),
	}, externalapi.AutomationsUpdateParams{ID: externalapi.EntityId(fixtures.AutomationJobsGreetingID)})
	require.NoError(t, err)
	require.IsType(t, &externalapi.AutomationResource{}, res)
}

// #748: a token with only the read and write scopes edits an active Automation's Steps, Trigger
// and Integration. The edit is a draft: the next Enrollment begins on the live version, the old
// Trigger still enrolls and the new one does not, and the mail is the live version's, through the
// live version's Integration (ADR 0065).
func TestAnEditWithoutThePublishScopeChangesNothingThatSends(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	editGreeting(t, env.ExternalScoped(t, "automations:read", "automations:write"))

	require.NoError(t, stepping.OnEvent(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactAliceID, automations.Event{Action: "job_hijacked", OccurredAt: time.Now()}, 0))
	assert.Empty(t, enrollmentsOf(t, env, fixtures.AutomationJobsGreetingID, fixtures.ContactAliceID), "the draft's Trigger enrolls nobody")
	runID := enrollVia(t, env, fixtures.AutomationJobsGreetingID, fixtures.ContactAliceID)
	run, err := env.DB.AutomationRun.Get(ctx, runID)
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.AutomationJobsGreetingVersionID, run.VersionID, "the Enrollment begins on the live version")

	fs := &fakeSender{}
	var through []int64
	env.RunQueuedSteps(t, stepperOf(env, recordingSenders(fs, &through)))

	assert.Equal(t, []string{"Hi Alice", "Hi Alice"}, subjectsOf(fs), "the live version's mail")
	assert.Equal(t, []int64{fixtures.IntegrationAcmeDefaultID, fixtures.IntegrationAcmeDefaultID}, through, "through the live version's Integration")
}

// After Publish with automations:publish, the next Enrollment follows the new version (its
// Trigger, Steps and Integration); an Enrollment already in flight stays on its version.
func TestAfterPublishTheNextEnrollmentFollowsTheNewVersion(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	inFlight := enrollVia(t, env, fixtures.AutomationJobsGreetingID, fixtures.ContactAliceID)
	editGreeting(t, env.ExternalScoped(t, "automations:read", "automations:write"))

	publisher := env.ExternalScoped(t, "automations:publish")
	res, err := publisher.AutomationsPublish(ctx, externalapi.AutomationsPublishParams{ID: externalapi.EntityId(fixtures.AutomationJobsGreetingID)})
	require.NoError(t, err)
	require.IsType(t, &externalapi.AutomationResource{}, res)

	require.NoError(t, stepping.OnEvent(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactBobID, automations.Event{Action: "job_hijacked", OccurredAt: time.Now()}, 0))
	next := enrollmentsOf(t, env, fixtures.AutomationJobsGreetingID, fixtures.ContactBobID)
	require.Len(t, next, 1, "the published Trigger enrolls")

	fs := &fakeSender{}
	var through []int64
	env.RunQueuedSteps(t, stepperOf(env, recordingSenders(fs, &through)))

	assert.ElementsMatch(t, []string{"Hi Alice", "Hi Alice", "Hijacked"}, subjectsOf(fs), "Alice finishes the old version, Bob gets the new one")
	assert.ElementsMatch(t, []int64{fixtures.IntegrationAcmeDefaultID, fixtures.IntegrationAcmeDefaultID, fixtures.IntegrationAcmeMailgunID}, through)
	stayed, err := env.DB.AutomationRun.Get(ctx, inFlight)
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.AutomationJobsGreetingVersionID, stayed.VersionID)
}
