package removalcheck_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/removalcheck"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A datetime Custom field a date Trigger names, in a draft not yet published, is in Definition use
// (ADR 0126): its retype to any type is refused, naming the Automation, beside the fixture
// Automation whose Wait until waits on the same field.
func TestACustomFieldADateTriggerNamesIsRefusedItsRetypeNamingTheAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	dated, err := testhelper.Module[*automations.Module](env).Create(ctx, env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{
		Name: "Trial ending",
		Trigger: automations.DateTrigger{Date: datetrigger.Date{
			Field:      datetrigger.Field{Kind: datetrigger.CustomField, Key: fixtures.CustomFieldAcmeTrialEndsAtKey},
			Recurrence: localcalendar.Once, OffsetDays: -3, LocalTime: "10:00",
		}},
	})
	require.NoError(t, err)

	refused := underLock(t, env, fixtures.AcmeID, func(s *ent.WorkspaceLocked) error {
		return testhelper.Module[*removalcheck.Module](env).CustomFieldRetype(ctx, s, fixtures.CustomFieldAcmeTrialEndsAtKey, customfield.TypeString)
	})

	assert.Equal(t, &liveuse.Error{What: liveuse.SubjectCustomField, Users: []liveuse.User{
		{Kind: liveuse.Automation, ID: fixtures.AutomationJobsWaitUntilDateID, Name: fixtures.AutomationJobsWaitUntilDateName},
		{Kind: liveuse.Automation, ID: dated.ID, Name: dated.Name},
	}}, refused)
}
