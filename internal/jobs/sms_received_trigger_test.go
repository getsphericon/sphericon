package jobs_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Automation whose Trigger is sms.received enrolls the Contact whose text reached the Twilio
// number, a Contact known by its phone alone, and its steps see the words. Fixture: the Jobs
// greeting Automation, its live version's Trigger turned to sms.received.
func TestAnAutomationTriggeredByATextEnrollsItsSender(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	greeting := env.DB.Automation.Query().Where(automation.ID(fixtures.AutomationJobsGreetingID)).OnlyX(ctx)
	env.DB.AutomationVersion.UpdateOneID(*greeting.LiveVersionID).SetTriggerEvent(events.NameSmsReceived).ExecX(ctx)
	sms := env.AcmeTwilio(t)
	res, err := env.ExternalScoped(t, "contacts:write").ContactsCreate(ctx, &externalapi.CreateContactInput{
		Phone: externalapi.NewOptNilString("+14155550188"),
	})
	require.NoError(t, err)
	texter, ok := res.(*externalapi.ContactResource)
	require.Truef(t, ok, "create the Contact: got %T", res)

	env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken, url.Values{
		"MessageSid": {"SM0100"}, "AccountSid": {sms.Account.SID}, "From": {"+14155550188"},
		"To": {testhelper.TwilioNumber}, "Body": {"YES"},
	})
	received := onlyEnvelope(t, env, events.NameSmsReceived)
	env.WorkAutomationJobOf(t, received)
	triggers := queuedArgs[jobs.EvaluateTriggerArgs](t, env, jobs.EvaluateTriggerArgs{}.Kind())
	require.Len(t, triggers, 1)
	require.NoError(t, jobs.NewEvaluateTriggerWorker(env.DB, env.Bus).Work(ctx, job(triggers[0])))

	run, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationJobsGreetingID), automationrun.ContactID(int64(texter.ID))).
		Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, events.NameSmsReceived, *run.EventAction)
	assert.Equal(t, "YES", run.EventProperties["text"])
}
