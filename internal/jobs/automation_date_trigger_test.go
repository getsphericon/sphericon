package jobs_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The date scheduler's tick (ADR 0126) queues the decision of each active date-triggered
// Automation, and the decision enrolls the Contacts whose occurrence came: a birthday at 10:00 is
// enrolled by the first tick after it, and ticks that come round again enroll no one twice.
func TestTheDateTickEnrollsTheContactsWhoseOccurrenceCame(t *testing.T) {
	clock := testhelper.NewClock(time.Date(2027, time.March, 13, 12, 0, 0, 0, time.UTC))
	env := testhelper.Setup(t, testhelper.WithClock(clock.Now))
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	alice, err := env.DB.Contact.Get(ctx, fixtures.ContactAliceID)
	require.NoError(t, err)
	_, err = testhelper.Module[*contacts.Module](env).Update(ctx, acme, alice.ID, contacts.UpdateInput{
		Revision: alice.Revision, Birthday: patch.Set(time.Date(1990, time.March, 14, 0, 0, 0, 0, time.UTC)),
	})
	require.NoError(t, err)
	m := testhelper.Module[*automations.Module](env)
	created, err := m.Create(ctx, acme, automations.CreateInput{Name: "Birthday", Trigger: automations.Trigger{Date: &datetrigger.Date{
		Field: datetrigger.Field{Kind: datetrigger.Birthday}, Recurrence: localcalendar.Yearly, LocalTime: "10:00",
	}}})
	require.NoError(t, err)
	_, err = m.Publish(ctx, acme, created.ID, testhelper.AutomationRevisionIn(ctx, t, acme, created.ID))
	require.NoError(t, err)

	clock.Set(time.Date(2027, time.March, 14, 10, 1, 0, 0, time.UTC))
	env.EnqueueJobs(t, jobs.DateTickArgs{}, jobs.DateTickArgs{})
	env.PerformEnqueuedJobs(t, jobs.DateTickArgs{}, jobs.DecideOccurrencesArgs{})
	clock.Set(time.Date(2027, time.March, 14, 10, 2, 0, 0, time.UTC))
	env.EnqueueJob(t, jobs.DateTickArgs{})
	env.PerformEnqueuedJobs(t, jobs.DateTickArgs{}, jobs.DecideOccurrencesArgs{})

	runs, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(created.ID), automationrun.ContactID(fixtures.ContactAliceID)).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, time.Date(2027, time.March, 14, 10, 0, 0, 0, time.UTC), runs[0].BeganAt.UTC())
}
