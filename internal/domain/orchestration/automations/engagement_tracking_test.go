package automations_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Destination's Engagement tracking consent is what the person decided, on every Channel (ADR
// 0120): its Events start, end and complete an Automation like any System event of what the
// person does.
func TestEngagementTrackingConsentTriggersExitsAndCompletes(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	pairs := [][2]string{
		{systemevent.EmailEngagementTrackingDenied, systemevent.EmailEngagementTrackingGranted},
		{systemevent.SmsEngagementTrackingDenied, systemevent.SmsEngagementTrackingGranted},
		{systemevent.TelegramEngagementTrackingDenied, systemevent.TelegramEngagementTrackingGranted},
	}

	for _, pair := range pairs {
		denied, granted := pair[0], pair[1]
		onRefusal, err := m.Create(t.Context(), s, automations.CreateInput{
			Name: "On " + denied, Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: denied}}, ExitEvents: []eventmatcher.Matcher{{Action: granted}},
		})
		require.NoError(t, err, denied)
		onGrant, err := m.Create(t.Context(), s, automations.CreateInput{
			Name: "On " + granted, Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: granted}}, ExitEvents: []eventmatcher.Matcher{},
			GoalEvent: &eventmatcher.Matcher{Action: denied},
		})
		require.NoError(t, err, granted)

		assert.NotZero(t, onRefusal.ID)
		assert.NotZero(t, onGrant.ID)
	}
}
