package jobs_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// sendLiamsAutomationEmail runs the email step Liam's Enrollment stands on and returns what went out.
func sendLiamsAutomationEmail(t *testing.T, env *testhelper.TestEnv) *fakeSender {
	t.Helper()
	queueStep(t, env, fixtures.AutomationRunJobsSingleEmailLiamID)
	fs := &fakeSender{}
	mod := outboundmessages.New(env.Bus, testhelper.Module[*holds.Holds](env), resolvingTo(fs), messagetoken.New("secret", "https://app.test", time.Now), env.OutboundModules(env.Bus, time.Now), outboundmessages.Deps{}, outboundmessages.Config{}, time.Now)
	env.RunQueuedSteps(t, stepperWith(env, mod))
	require.Len(t, fs.sent, 1)
	return fs
}

// An Automation email to a Destination with no consent is tracked under the default track.
func TestAnAutomationEmailIsTrackedUnderTheDefault(t *testing.T) {
	env := testhelper.Setup(t)

	fs := sendLiamsAutomationEmail(t, env)

	assert.Contains(t, fs.sent[0].HTML, "https://app.test/e/o/", "the open pixel")
}

// An Automation email to a Destination that refused tracking carries no pixel, and keeps its
// unsubscribe footer and List-Unsubscribe (ADR 0120).
func TestAnAutomationEmailToADestinationThatRefusedTrackingIsNotTracked(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := env.ExternalScoped(t, apitokens.ScopeContactsWrite).EngagementTrackingCreate(t.Context(), &externalapi.RecordEngagementTrackingInput{
		Channel: externalapi.EngagementTrackingChannelEmail, Destination: fixtures.ContactLiamEmail, State: externalapi.EngagementTrackingStateDenied,
	})
	require.NoError(t, err)

	fs := sendLiamsAutomationEmail(t, env)

	assert.NotContains(t, fs.sent[0].HTML, "/e/o/", "no open pixel")
	assert.Contains(t, fs.sent[0].HTML, "https://app.test/e/u/", "the unsubscribe footer stays")
	assert.Contains(t, fs.sent[0].ListUnsubscribeURL, "https://app.test/e/u/", "and List-Unsubscribe")
}
