package removalcheck_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/removalcheck"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A datetime Custom field a Wait until waits on (ADR 0126), in a live version or a draft, is in
// Definition use: a retype to any other type is refused, naming the Automations, since the wait
// would never come.
func TestACustomFieldAWaitUntilNamesIsRefusedARetypeNamingTheAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	waits := automationstep.Root(automationstep.New(automationstep.WaitUntil{Until: automationstep.OnDate{Date: datetrigger.Date{
		Field:      datetrigger.Field{Kind: datetrigger.CustomField, Key: fixtures.CustomFieldAcmeTrialEndsAtKey},
		Recurrence: localcalendar.Once, OffsetDays: -3, LocalTime: "10:00",
	}}}))
	live := branchingOn(t, env, fixtures.AcmeID, "Trial ending", waits, true)
	draft := branchingOn(t, env, fixtures.AcmeID, "Draft trial ending", waits, false)

	refused := underLock(t, env, fixtures.AcmeID, func(s *ent.WorkspaceLocked) error {
		return testhelper.Module[*removalcheck.Module](env).CustomFieldRetype(t.Context(), s, fixtures.CustomFieldAcmeTrialEndsAtKey, customfield.TypeString)
	})

	assert.Equal(t, &liveuse.Error{What: liveuse.SubjectCustomField, Users: []liveuse.User{
		{Kind: liveuse.Automation, ID: fixtures.AutomationJobsWaitUntilDateID, Name: fixtures.AutomationJobsWaitUntilDateName},
		{Kind: liveuse.Automation, ID: live.ID, Name: live.Name},
		{Kind: liveuse.Automation, ID: draft.ID, Name: draft.Name},
	}}, refused)
}
