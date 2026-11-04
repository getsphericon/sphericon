package jobs_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Automation whose Trigger is telegram.received enrolls the Contact whose message the bot read,
// and its steps see the words. Fixture: the Jobs greeting Automation, its live version's Trigger
// turned to telegram.received.
func TestAnAutomationTriggeredByATelegramMessageEnrollsItsSender(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	greeting := env.DB.Automation.Query().Where(automation.ID(fixtures.AutomationJobsGreetingID)).OnlyX(ctx)
	env.DB.AutomationVersion.UpdateOneID(*greeting.LiveVersionID).SetTriggerEvent(events.NameTelegramReceived).ExecX(ctx)
	bot := env.ConnectTelegram(t, 7401, "trigger_bot")
	env.BindTelegram(t, bot, fixtures.ContactLiamID, 4101, 1000)

	env.PostTelegramUpdate(t, bot, `{"update_id":1,"message":{"message_id":1,"date":1767225600,"chat":{"id":4101,"type":"private"},"from":{"id":4101,"is_bot":false},"text":"hi"}}`)
	received := onlyEnvelope(t, env, events.NameTelegramReceived)
	env.WorkAutomationJobOf(t, received)
	triggers := queuedArgs[jobs.EvaluateTriggerArgs](t, env, jobs.EvaluateTriggerArgs{}.Kind())
	require.Len(t, triggers, 1)
	require.NoError(t, jobs.NewEvaluateTriggerWorker(env.DB, env.Bus).Work(ctx, job(triggers[0])))

	run, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationJobsGreetingID), automationrun.ContactID(fixtures.ContactLiamID)).
		Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, events.NameTelegramReceived, *run.EventAction)
	assert.Equal(t, "hi", run.EventProperties["text"])
}
