package testhelper

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
)

// OnceDateEnrollment makes Alice's Enrollment in an Automation on a once date Trigger (ADR 0126) as
// the date scheduler makes it, on an env set up WithClock(clock.Now), and returns the Automation's
// id. It publishes "Trial ending" on Acme's trial_ends_at at 10:00, sets her date to the day after
// the clock's, moves the clock five minutes past that occurrence and decides it. No fixture holds
// such an Enrollment: a version naming trial_ends_at would hold the field in Definition use for
// every test that retypes it.
func (env *TestEnv) OnceDateEnrollment(t *testing.T, clock *Clock) int64 {
	t.Helper()
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	m := Module[*automations.Module](env)
	created, err := m.Create(ctx, acme, automations.CreateInput{Name: "Trial ending", Trigger: automations.Trigger{Date: &datetrigger.Date{
		Field:      datetrigger.Field{Kind: datetrigger.CustomField, Key: fixtures.CustomFieldAcmeTrialEndsAtKey},
		Recurrence: localcalendar.Once, LocalTime: "10:00",
	}}})
	require.NoError(t, err)
	_, err = m.Publish(ctx, acme, created.ID, AutomationRevisionIn(ctx, t, acme, created.ID))
	require.NoError(t, err)
	alice, err := env.DB.Contact.Get(ctx, fixtures.ContactAliceID)
	require.NoError(t, err)
	day := clock.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
	_, err = Module[*contacts.Module](env).Update(ctx, acme, alice.ID, contacts.UpdateInput{
		Revision:     alice.Revision,
		CustomFields: patch.Set(map[string]any{fixtures.CustomFieldAcmeTrialEndsAtKey: day.Format(time.DateOnly)}),
	})
	require.NoError(t, err)
	clock.Set(day.Add(10*time.Hour + 5*time.Minute))
	require.NoError(t, Module[*automations.StepExecution](env).DecideOccurrences(ctx, acme, created.ID))
	return created.ID
}
