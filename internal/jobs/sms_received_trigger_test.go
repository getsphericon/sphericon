package jobs_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Automation whose Trigger is sms.received enrolls the Contact whose text reached the Twilio
// number, a Contact known by its phone alone, and its steps see the words. Fixture: the Automation
// the fixtures trigger on sms.received.
func TestAnAutomationTriggeredByATextEnrollsItsSender(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	sms := env.AcmeTwilio(t)
	res, err := env.ExternalScoped(t, apitokens.ScopeContactsWrite).ContactsCreate(ctx, &externalapi.CreateContactInput{
		Phone: externalapi.NewOptNilString("+14155550188"),
	})
	require.NoError(t, err)
	texter := res

	env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken, url.Values{
		"MessageSid": {"SM0100"}, "AccountSid": {sms.Account.SID}, "From": {"+14155550188"},
		"To": {testhelper.TwilioNumber}, "Body": {"YES"},
	})
	onlyEnvelope(t, env, systemevent.SmsReceived)
	triggers := triggersOn(t, env, systemevent.SmsReceived)
	require.Len(t, triggers, 1)
	require.NoError(t, jobs.NewEvaluateTriggerWorker(env.DB, env.StepExecution).Work(ctx, job(triggers[0])))

	run, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationOnSmsReceivedID), automationrun.ContactID(int64(texter.ID))).
		Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, systemevent.SmsReceived, *run.EventAction)
	assert.Equal(t, "YES", run.EventProperties["text"])
}
