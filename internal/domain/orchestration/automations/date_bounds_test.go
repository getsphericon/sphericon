package automations_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A date out of the contract's bounds is refused as any bound is (ADR 0107), on the request field
// that holds it: a date Trigger's offset beyond a year under `trigger`, and a Wait until's local time
// not written HH:MM under its Step's `until`.
func TestADateOutOfItsBoundsIsRefusedOnItsField(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	m := testhelper.Module[*automations.Module](env)
	acme := env.DB.Scoped(fixtures.AcmeID)
	wait := automationstep.New(automationstep.WaitUntil{Until: automationstep.OnDate{Date: datetrigger.Date{
		Field: datetrigger.Field{Kind: datetrigger.Birthday}, Recurrence: localcalendar.Yearly, LocalTime: "9:00",
	}}})

	_, trigger := m.Create(ctx, acme, automations.CreateInput{Name: "A year and a day", Trigger: automations.DateTrigger{Date: datetrigger.Date{
		Field: datetrigger.Field{Kind: datetrigger.Birthday}, Recurrence: localcalendar.Yearly, OffsetDays: 366, LocalTime: "10:00",
	}}})
	_, step := m.Create(ctx, acme, automations.CreateInput{
		Name: "At nine", Trigger: automations.DateTrigger{Date: birthday(0, "10:00")}, Steps: new(automationstep.Root(wait)),
	})

	require.ErrorIs(t, trigger, validation.ErrOutOfRange)
	triggerField, ok := errors.AsType[*validation.RefusalError](trigger)
	require.True(t, ok)
	assert.Equal(t, "trigger.offsetDays", triggerField.Field())
	require.ErrorIs(t, step, validation.ErrPatternMismatch)
	stepField, ok := errors.AsType[*validation.RefusalError](step)
	require.True(t, ok)
	assert.Equal(t, "steps.steps.0.until.localTime", stepField.Field())
}
