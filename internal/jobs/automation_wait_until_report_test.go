package jobs_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
)

// The report counts, for the Wait until's Step, the Enrollments that went on at once in the
// period, by reason (past, no_date) and those beyond_bound apart. Liam's and Alice's trials have
// ended, Bob holds no trial_ends_at and Carol's ends two years ahead.
func TestTheReportCountsTheEnrollmentsAWaitUntilLetGoOnByReason(t *testing.T) {
	newYork := zoneOf(t, "America/New_York")
	env, clock := waitUntilEnv(t, time.Date(2027, time.January, 15, 12, 0, 0, 0, newYork))
	for contactID, fields := range map[int64]map[string]any{
		fixtures.ContactLiamID:  {"trial_ends_at": "2027-01-10"},
		fixtures.ContactAliceID: {"trial_ends_at": "2026-12-01"},
		fixtures.ContactBobID:   {},
		fixtures.ContactCarolID: {"trial_ends_at": "2029-01-20"},
	} {
		updateContact(t, env, contactID, contacts.UpdateInput{CustomFields: patch.Set(fields)})
		enrollOnItsTrigger(t, env, clock, fixtures.AutomationJobsWaitUntilDateID, contactID)
	}
	stepper := stepperAt(env, newMod(env, resolvingTo(&fakeSender{}), clock.read), clock.read)
	env.RunQueuedSteps(t, stepper)

	report := automationReport(t, env, fixtures.AutomationJobsWaitUntilDateID)

	assert.Equal(t, []externalapi.AutomationWaitUntilFigures{
		{StepId: "9a3c1e52-5592-4b6e-8a1d-6f2c3b4a5d02", Past: 2, NoDate: 1, BeyondBound: 1},
	}, report.WaitUntilSteps)
}
