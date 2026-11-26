package jobs_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Automation whose Trigger is telegram.received enrolls the Contact whose message the bot read,
// and its steps see the words. Fixture: the Automation the fixtures trigger on telegram.received.
func TestAnAutomationTriggeredByATelegramMessageEnrollsItsSender(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactLiamID, 4101, 1000)

	env.PostTelegramUpdate(t, bot, testhelper.TelegramMessage(1, 4101, "hi"))
	onlyEnvelope(t, env, systemevent.TelegramReceived)
	triggers := triggersOn(t, env, systemevent.TelegramReceived)
	require.Len(t, triggers, 1)
	require.NoError(t, jobs.NewEvaluateTriggerWorker(env.DB, testhelper.Module[*automations.StepExecution](env)).Work(ctx, job(triggers[0])))

	run, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationOnTelegramReceivedID), automationrun.ContactID(fixtures.ContactLiamID)).
		Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, systemevent.TelegramReceived, *run.EventAction)
	assert.Equal(t, "hi", run.EventProperties["text"])
}
